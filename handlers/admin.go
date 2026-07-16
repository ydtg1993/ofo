package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ofo/logger"
	"ofo/middleware"
	"ofo/models"
	"ofo/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

// sanitizePolicy returns a bluemonday policy that allows HTML and video elements.
func sanitizePolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// 允许视频元素
	p.AllowElements("video", "source")
	p.AllowAttrs("src", "controls", "width", "height", "autoplay", "loop", "muted", "poster").OnElements("video")
	p.AllowAttrs("src", "type").OnElements("source")
	// 允许常用 HTML 布局标签 + 样式/类名
	p.AllowElements("div", "span", "section", "article", "header", "footer", "nav", "aside", "main", "figure", "figcaption", "details", "summary")
	p.AllowAttrs("class", "id", "style").OnElements("div", "span", "section", "article", "header", "footer", "nav", "aside", "main", "figure", "figcaption", "details", "summary")
	// 允许图片宽高和样式
	p.AllowAttrs("width", "height", "style").OnElements("img")
	p.AllowAttrs("class", "id", "style").Globally()
	// 允许 iframe（嵌入视频等）
	p.AllowElements("iframe")
	p.AllowAttrs("src", "width", "height", "frameborder", "allowfullscreen", "allow", "style").OnElements("iframe")
	return p
}

// renderMarkdown converts markdown to sanitized HTML.
func renderMarkdown(md string) string {
	// 预处理：确保块引用、标题、列表前有空行
	md = normalizeMarkdown(md)
	// 预处理：递归渲染 HTML 容器标签内的 Markdown（如 <div>![](url)</div>）
	md = renderHTMLContainers(md)
	unsafe := blackfriday.Run([]byte(md))
	html := string(sanitizePolicy().SanitizeBytes(unsafe))
	// 给正文图片加懒加载
	html = InjectLazyLoading(html)
	return html
}

// reImgTag matches an <img> tag body for lazy-load injection.
var reImgTag = regexp.MustCompile(`<img\s([^>]*?)>`)

// InjectLazyLoading adds loading="lazy" to every <img> that lacks a loading attribute.
// Exported so the router's template function can reuse it for existing posts.
func InjectLazyLoading(html string) string {
	return reImgTag.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "loading=") {
			return match
		}
		return strings.Replace(match, "<img ", "<img loading=\"lazy\" ", 1)
	})
}

// ---- Image Dimension Injection (lazy loading fix) ----

// reImgSrc extracts the src attribute value from an <img> tag.
var reImgSrc = regexp.MustCompile(`src\s*=\s*"([^"]*)"`)

// InjectImageDimensions adds width and height attributes to <img> tags
// whose src points to a storage-managed file, reading dimensions via the
// Storage interface. External URLs and data URIs are skipped.
func InjectImageDimensions(html string, store storage.Storage) string {
	return reImgTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src
		m := reImgSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process storage-managed URLs
		if !store.IsStorageURL(src) {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		w, h, err := store.GetMediaInfo(src)
		if err != nil || w == 0 || h == 0 {
			// Unreadable or not media -- leave tag untouched
			return match
		}

		return injectWidthHeight(match, w, h)
	})
}

// injectWidthHeight adds width, height, and aspect-ratio attributes into an img/video tag.
// The aspect-ratio inline style ensures the browser reserves the correct space
// even when src is removed by the lazy-loading script (e.g. slow network).
func injectWidthHeight(tag string, w, h int) string {
	// Build aspect-ratio inline style — this is the key to preventing layout shift
	ratioStyle := fmt.Sprintf("aspect-ratio:%d/%d", w, h)

	// Merge with existing style attribute if present
	if idx := strings.Index(tag, "style=\""); idx >= 0 {
		// Find closing quote of existing style value
		closeQuote := strings.Index(tag[idx+7:], "\"")
		if closeQuote >= 0 {
			tag = tag[:idx+7+closeQuote] + ";" + ratioStyle + tag[idx+7+closeQuote:]
		}
	} else {
		// Insert style attribute before closing >
		styleAttr := fmt.Sprintf(` style="%s"`, ratioStyle)
		if strings.HasSuffix(tag, "/>") {
			tag = tag[:len(tag)-2] + styleAttr + "/>"
		} else {
			tag = tag[:len(tag)-1] + styleAttr + ">"
		}
	}

	// Add width/height HTML attributes
	dims := fmt.Sprintf(` width="%d" height="%d"`, w, h)
	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + dims + "/>"
	}
	return tag[:len(tag)-1] + dims + ">"
}

// ---- Video Dimension Injection ----

// reVideoTag matches a <video> opening tag.
var reVideoTag = regexp.MustCompile(`<video\s([^>]*?)>`)

// reVideoSrc extracts the src attribute value from a <video> tag.
var reVideoSrc = regexp.MustCompile(`src\s*=\s*"([^"]*)"`)

// InjectVideoDimensions adds width and height attributes to <video> tags
// whose src points to a storage-managed file, reading dimensions via the
// Storage interface. External URLs and data URIs are skipped.
func InjectVideoDimensions(html string, store storage.Storage) string {
	return reVideoTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src from the opening <video> tag
		m := reVideoSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process storage-managed URLs
		if !store.IsStorageURL(src) {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		w, h, err := store.GetMediaInfo(src)
		if err != nil || w == 0 || h == 0 {
			// Unreadable or not media -- leave tag untouched
			return match
		}

		return injectWidthHeight(match, w, h)
	})
}

// ThumbnailImage generates an <img> tag for a thumbnail URL with skeleton-ready
// attributes (width, height, aspect-ratio) injected for storage-managed files.
// Used by the homepage card thumbnails.
func ThumbnailImage(url, alt string, store storage.Storage) string {
	if url == "" {
		return ""
	}
	html := fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy">`, url, alt)
	html = InjectImageDimensions(html, store)
	return html
}

// HTML 容器标签集合
var htmlContainerTags = map[string]bool{
	"div": true, "section": true, "article": true, "figure": true,
	"figcaption": true, "details": true, "summary": true,
	"header": true, "footer": true, "nav": true, "aside": true, "main": true,
}

// reHTMLContainer 匹配 HTML 容器标签，捕获标签名、属性和内容。
var reHTMLContainer = regexp.MustCompile(
	`(?s)<(div|section|article|figure|figcaption|details|summary|header|footer|nav|aside|main)\b([^>]*)>(.+?)</(\w+)>`,
)

// renderHTMLContainers 递归渲染 HTML 容器内的 Markdown 内容。
func renderHTMLContainers(md string) string {
	for i := 0; i < 10; i++ {
		before := md
		md = reHTMLContainer.ReplaceAllStringFunc(md, func(match string) string {
			sub := reHTMLContainer.FindStringSubmatch(match)
			if len(sub) < 5 {
				return match
			}
			openTag := sub[1]
			attrs := sub[2]
			content := sub[3]
			closeTag := sub[4]

			// 只处理首尾标签匹配的
			if openTag != closeTag || !htmlContainerTags[openTag] {
				return match
			}

			// 把 width="100px" 等属性转为内联 style
			attrs = normalizeHTMLAttrs(attrs)

			rendered := blackfriday.Run([]byte(content))
			return "<" + openTag + attrs + ">\n" + string(rendered) + "\n</" + openTag + ">"
		})
		if md == before {
			break
		}
	}
	return md
}

// reAttrWidth 匹配模板中直接写的 width="100px" / height="200" 等属性
var reAttrWidth = regexp.MustCompile(`(?i)\b(width|height)\s*=\s*"(\d+%?)"`)
var reAttrAlign = regexp.MustCompile(`(?i)\b(align)\s*=\s*"(left|center|right)"`)

// normalizeHTMLAttrs 把 width/height/align 等 HTML 废弃属性转为 inline style。
func normalizeHTMLAttrs(attrs string) string {
	attrs = reAttrWidth.ReplaceAllStringFunc(attrs, func(m string) string {
		parts := reAttrWidth.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		prop := strings.ToLower(parts[1])
		val := parts[2]
		return "style=\"" + prop + ":" + val + "\""
	})
	attrs = reAttrAlign.ReplaceAllStringFunc(attrs, func(m string) string {
		parts := reAttrAlign.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return "style=\"text-align:" + strings.ToLower(parts[2]) + "\""
	})
	return attrs
}

// normalizeMarkdown 预处理 markdown，让非空行前的 > / # / - 能正确解析。
var reBlockNeedsBlank = regexp.MustCompile(`(?m)^([^\n>#\-\s].+)\n(> |#{1,6} |\d+\. |\- )`)

func normalizeMarkdown(md string) string {
	// 统一换行符：\r\n (Windows) / \r (old Mac) → \n
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	// 在块级元素前补空行（如果前一非空行不是空行或另一个块元素）
	return reBlockNeedsBlank.ReplaceAllString(md, "$1\n\n$2")
}

// AdminPageData holds data for admin template rendering.
type AdminPageData struct {
	Title          string
	Cfg            interface{}
	Error          string
	Success        string
	Posts          []models.Post
	Post           *models.Post
	Categories     []models.Category
	Tags           []models.Tag
	AllTags        []models.Tag
	IsEditing      bool
	IsNew          bool
	ShowCategories bool
	ShowStickers   bool
	Stickers       []models.Sticker
	Pagination     *models.Pagination
}

// ---- Login ----

func (h *Handler) AdminLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_login.html", gin.H{
		"Cfg":   h.Cfg,
		"Error": "",
	})
}

func (h *Handler) AdminLogin(c *gin.Context) {
	password := c.PostForm("password")
	if password != h.Cfg.AdminPassword {
		c.HTML(http.StatusUnauthorized, "admin_login.html", gin.H{
			"Cfg":   h.Cfg,
			"Error": "密码错误",
		})
		return
	}
	middleware.SetAdminCookie(c, h.Cfg.AdminPassword)
	c.Redirect(http.StatusFound, "/admin")
}

// ---- Logout ----

func (h *Handler) AdminLogout(c *gin.Context) {
	middleware.ClearAdminCookie(c)
	c.Redirect(http.StatusFound, "/admin/login")
}

// ---- Dashboard ----

func (h *Handler) AdminDashboard(c *gin.Context) {
	total, err := h.PostModel.CountAll()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count posts", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage

	posts, err := h.PostModel.ListAllPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list paginated posts", "err", err)
		c.HTML(http.StatusInternalServerError, "admin_dashboard.html", AdminPageData{
			Title: "Dashboard",
			Cfg:   h.Cfg,
			Error: "加载文章列表失败",
		})
		return
	}

	categories, err := h.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for dashboard", "err", err)
	}

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Pagination: pg,
	})
}

// ---- New Post Form ----

func (h *Handler) AdminNewPost(c *gin.Context) {
	categories, err := h.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for new post", "err", err)
	}
	allTags, err := h.PostModel.AllTagsSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for new post", "err", err)
	}

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:      "New Post",
		Cfg:        h.Cfg,
		IsNew:      true,
		Categories: categories,
		AllTags:    allTags,
	})
}

// ---- Edit Post Form ----

func (h *Handler) AdminEditPost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	post, err := h.PostModel.GetByID(id)
	if err != nil {
		logger.WarnWithContext(c, "failed to get post for edit", "id", id, "err", err)
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	categories, err := h.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for edit", "err", err)
	}
	tags, err := h.PostModel.TagsForPost(id)
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for edit", "postID", id, "err", err)
	}
	allTags, err := h.PostModel.AllTagsSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load all tags for edit", "err", err)
	}

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:      "Edit: " + post.Title,
		Cfg:        h.Cfg,
		Post:       post,
		Categories: categories,
		Tags:       tags,
		AllTags:    allTags,
		IsEditing:  true,
	})
}

// ---- Create Post ----

func (h *Handler) AdminCreatePost(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagStr := c.PostForm("tags")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	// Render markdown
	contentHTML := renderMarkdown(contentMD)

	// Excerpt
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = extractExcerptStr(contentMD, 200)
	}

	// Auto-extract thumbnail if not manually set
	if thumbnailURL == "" {
		thumbnailURL = models.ExtractThumbnail(contentHTML)
	}

	var categoryID sql.NullInt64
	if categoryIDStr != "" {
		if cid, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			categoryID = sql.NullInt64{Int64: cid, Valid: true}
		}
	}

	tagNames := parseTags(tagStr)

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))

	postID, err := h.PostModel.Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, createdAt, tagNames)
	if err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "New Post",
			Cfg:        h.Cfg,
			IsNew:      true,
			Categories: categories,
			Error:      "保存失败：" + err.Error(),
		})
		return
	}

	// 关联上传资源到文章
	if err := h.ResourceModel.SyncPostResources(int(postID), contentHTML, h.Storage.IsStorageURL, func(filename string) error {
		return h.Storage.Delete(c.Request.Context(), "uploads/"+filename)
	}); err != nil {
		logger.ErrorWithContext(c, "failed to sync resources for new post", "postID", postID, "err", err)
	}

	// Redirect to dashboard with success
	h.adminDashboardWithSuccess(c, "文章发布成功")
}

// ---- Update Post ----

func (h *Handler) AdminUpdatePost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagStr := c.PostForm("tags")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	contentHTML := renderMarkdown(contentMD)
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = extractExcerptStr(contentMD, 200)
	}

	// Auto-extract thumbnail if not manually set
	if thumbnailURL == "" {
		thumbnailURL = models.ExtractThumbnail(contentHTML)
	}

	var categoryID sql.NullInt64
	if categoryIDStr != "" {
		if cid, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			categoryID = sql.NullInt64{Int64: cid, Valid: true}
		}
	}

	tagNames := parseTags(tagStr)

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))

	if err := h.PostModel.Update(id, title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, createdAt, tagNames); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		tags, _ := h.PostModel.TagsForPost(id)
		post, _ := h.PostModel.GetByID(id)
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "Edit: " + title,
			Cfg:        h.Cfg,
			Post:       post,
			Categories: categories,
			Tags:       tags,
			IsEditing:  true,
			Error:      "更新失败：" + err.Error(),
		})
		return
	}

	// 同步上传资源关联
	if err := h.ResourceModel.SyncPostResources(id, contentHTML, h.Storage.IsStorageURL, func(filename string) error {
		return h.Storage.Delete(c.Request.Context(), "uploads/"+filename)
	}); err != nil {
		logger.ErrorWithContext(c, "failed to sync resources for updated post", "postID", id, "err", err)
	}

	h.adminDashboardWithSuccess(c, "文章更新成功")
}

// ---- Delete Post ----

func (h *Handler) AdminDeletePost(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// 1. 查找文章关联的资源
	resources, err := h.ResourceModel.FindByPostID(id)
	if err != nil {
		logger.ErrorWithContext(c, "failed to find resources for post deletion", "postID", id, "err", err)
	}

	// 2. 删除存储中的资源文件
	for _, r := range resources {
		key := "uploads/" + r.Filename
		if err := h.Storage.Delete(c.Request.Context(), key); err != nil {
			logger.ErrorWithContext(c, "failed to delete resource file", "filename", r.Filename, "err", err)
		}
	}

	// 3. 删除资源记录
	if err := h.ResourceModel.DeleteByPostID(id); err != nil {
		logger.ErrorWithContext(c, "failed to delete resource records", "postID", id, "err", err)
	}

	// 4. 删除文章本身
	if err := h.PostModel.Delete(id); err != nil {
		h.adminDashboardWithSuccess(c, "删除文章失败")
		return
	}

	h.adminDashboardWithSuccess(c, "文章删除成功")
}

// ---- Category Management ----

func (h *Handler) AdminCategories(c *gin.Context) {
	categories, err := h.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for management", "err", err)
	}

	c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
		Title:          "Category Management",
		Cfg:            h.Cfg,
		Categories:     categories,
		ShowCategories: true,
	})
}

func (h *Handler) AdminCreateCategory(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}
	if name == "" {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	if err := h.PostModel.CreateCategory(name, slug); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "创建分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (h *Handler) AdminUpdateCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}

	if err := h.PostModel.UpdateCategory(id, name, slug); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "更新分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (h *Handler) AdminDeleteCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := h.PostModel.DeleteCategory(id); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "删除分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

// ---- Sticker Management ----

// AdminStickers displays the sticker management page.
func (h *Handler) AdminStickers(c *gin.Context) {
	total, err := h.StickerModel.Count()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count stickers", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage

	stickers, err := h.StickerModel.ListPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list stickers", "err", err)
	}

	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
		Stickers:     stickers,
		Pagination:   pg,
	})
}

// AdminCreateSticker handles sticker upload (single file per request, supports AJAX).
func (h *Handler) AdminCreateSticker(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.stickerError(c, "请选择文件")
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mov": true,
	}
	if !allowed[ext] {
		h.stickerError(c, "不支持的文件类型："+ext)
		return
	}

	// Generate unique filename with date-based folder prefix (e.g. 2026/07/<uuid>.ext)
	savedName := time.Now().Format("2006/01") + "/" + uuid.New().String() + ext

	// Upload via storage backend
	key := "stickers/" + savedName
	url, err := h.Storage.Upload(c.Request.Context(), key, file, header.Size)
	if err != nil {
		logger.ErrorWithContext(c, "failed to upload sticker", "name", savedName, "err", err)
		h.stickerError(c, "保存文件失败")
		return
	}
	mimeType := models.MIMEType(ext)
	if _, err := h.StickerModel.Create(savedName, url, header.Size, mimeType); err != nil {
		logger.ErrorWithContext(c, "failed to record sticker in database", "name", savedName, "err", err)
	}

	// AJAX request → JSON response; regular form → redirect
	if c.GetHeader("X-Requested-With") == "XMLHttpRequest" || c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{"url": url, "filename": savedName})
		return
	}
	c.Redirect(http.StatusFound, "/admin/stickers")
}

// stickerError returns an error for the sticker management page (HTML or JSON).
func (h *Handler) stickerError(c *gin.Context, msg string) {
	if c.GetHeader("X-Requested-With") == "XMLHttpRequest" || c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	total, err := h.StickerModel.Count()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count stickers", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage
	stickers, err := h.StickerModel.ListPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list stickers", "err", err)
	}

	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
		Stickers:     stickers,
		Pagination:   pg,
		Error:        msg,
	})
}

// AdminDeleteSticker removes a sticker by ID and deletes the file from disk.
func (h *Handler) AdminDeleteSticker(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/stickers")
		return
	}

	// 先查出记录，拿到文件名
	sticker, err := h.StickerModel.GetByID(id)
	if err != nil {
		logger.WarnWithContext(c, "sticker not found for deletion", "stickerID", id, "err", err)
		c.Redirect(http.StatusFound, "/admin/stickers")
		return
	}

	// 删除存储中的文件
	key := "stickers/" + sticker.Filename
	if err := h.Storage.Delete(c.Request.Context(), key); err != nil {
		logger.ErrorWithContext(c, "failed to delete sticker file", "filename", sticker.Filename, "err", err)
	}

	// 删除数据库记录
	if err := h.StickerModel.Delete(id); err != nil {
		logger.ErrorWithContext(c, "failed to delete sticker record", "stickerID", id, "err", err)
	}

	c.Redirect(http.StatusFound, "/admin/stickers")
}

// ---- File Upload ----

func (h *Handler) AdminUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mov": true,
	}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件类型：" + ext})
		return
	}

	// Generate unique filename with date-based folder prefix (e.g. 2026/07/<uuid>.ext)
	savedName := time.Now().Format("2006/01") + "/" + uuid.New().String() + ext

	// Upload via storage backend
	key := "uploads/" + savedName
	url, err := h.Storage.Upload(c.Request.Context(), key, file, header.Size)
	if err != nil {
		logger.ErrorWithContext(c, "failed to upload file", "name", savedName, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 记录到资源表（post_id 暂时为空，保存文章时关联）
	mimeType := models.MIMEType(ext)
	if _, err := h.ResourceModel.Create(savedName, url, header.Size, mimeType); err != nil {
		logger.ErrorWithContext(c, "failed to record uploaded resource in database", "name", savedName, "err", err)
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ---- Helpers ----

func (h *Handler) adminDashboardWithSuccess(c *gin.Context, msg string) {
	total, err := h.PostModel.CountAll()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count posts for dashboard", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage
	posts, err := h.PostModel.ListAllPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list posts for dashboard", "err", err)
	}
	categories, err := h.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for dashboard", "err", err)
	}

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Success:    msg,
		Pagination: pg,
	})
}

// adminPagination builds Pagination info from query params.
func adminPagination(c *gin.Context, total int, perPage int) *models.Pagination {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return &models.Pagination{
		CurrentPage: page,
		TotalPages:  totalPages,
		PerPage:     perPage,
		TotalPosts:  total,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}
}

func slugifyStr(s string) string {
	result := ""
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result += strings.ToLower(string(r))
		} else if r == ' ' || r == '-' || r == '_' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
			}
		}
	}
	slug := strings.Trim(result, "-")
	// 兜底：纯中文等标题 slugify 后为空时，用时间戳
	if slug == "" {
		slug = fmt.Sprintf("post-%d", time.Now().Unix())
	}
	return slug
}

func extractExcerptStr(md string, maxLen int) string {
	// Strip markdown roughly
	replacer := strings.NewReplacer("`", "", "#", "", "*", "", "_", "", "[", "", "]", "", "(", "", ")", "")
	clean := replacer.Replace(md)
	// Remove ``` blocks
	for {
		start := strings.Index(clean, "```")
		if start < 0 {
			break
		}
		end := strings.Index(clean[start+3:], "```")
		if end < 0 {
			break
		}
		clean = clean[:start] + clean[start+3+end+3:]
	}
	clean = strings.Join(strings.Fields(clean), " ")

	if len(clean) > maxLen {
		cut := clean[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return clean
}

func parseTags(tagStr string) []string {
	if strings.TrimSpace(tagStr) == "" {
		return nil
	}
	// 按换行拆分，兼容 \n 和 \r\n
	parts := strings.FieldsFunc(tagStr, func(r rune) bool { return r == '\n' || r == '\r' })
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// parseDate parses a form date string, defaults to today.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t
		}
	}
	return time.Now()
}
