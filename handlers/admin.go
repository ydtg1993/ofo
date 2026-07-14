package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ofo/middleware"
	"ofo/models"

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
	p.AllowAttrs("class", "id").Globally()
	// 允许 iframe（嵌入视频等）
	p.AllowElements("iframe")
	p.AllowAttrs("src", "width", "height", "frameborder", "allowfullscreen", "allow", "style").OnElements("iframe")
	return p
}

// renderMarkdown converts markdown to sanitized HTML.
func renderMarkdown(md string) string {
	// 预处理：确保块引用、标题、列表前有空行（非标准 markdown 的宽松模式）
	md = normalizeMarkdown(md)
	unsafe := blackfriday.Run([]byte(md))
	return string(sanitizePolicy().SanitizeBytes(unsafe))
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
	posts, err := h.PostModel.ListAll()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "admin_dashboard.html", AdminPageData{
			Title: "Dashboard",
			Cfg:   h.Cfg,
			Error: "加载文章列表失败",
		})
		return
	}

	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
	})
}

// ---- New Post Form ----

func (h *Handler) AdminNewPost(c *gin.Context) {
	categories, _ := h.PostModel.AllCategoriesSimple()
	allTags, _ := h.PostModel.AllTagsSimple()

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
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	categories, _ := h.PostModel.AllCategoriesSimple()
	tags, _ := h.PostModel.TagsForPost(id)
	allTags, _ := h.PostModel.AllTagsSimple()

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
	if err := h.ResourceModel.SyncPostResources(int(postID), contentHTML); err != nil {
		log.Printf("AdminCreatePost: failed to sync resources for post %d: %v", postID, err)
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
	if err := h.ResourceModel.SyncPostResources(id, contentHTML); err != nil {
		log.Printf("AdminUpdatePost: failed to sync resources for post %d: %v", id, err)
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
		log.Printf("AdminDeletePost: failed to find resources for post %d: %v", id, err)
	}

	// 2. 删除磁盘上的资源文件
	uploadsDir := filepath.Join(h.BaseDir, "static", "uploads")
	for _, r := range resources {
		filePath := filepath.Join(uploadsDir, r.Filename)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("AdminDeletePost: failed to delete file %s: %v", filePath, err)
		}
	}

	// 3. 删除资源记录
	if err := h.ResourceModel.DeleteByPostID(id); err != nil {
		log.Printf("AdminDeletePost: failed to delete resource records for post %d: %v", id, err)
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
	categories, _ := h.PostModel.AllCategoriesSimple()

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
	stickers, _ := h.StickerModel.ListAll()

	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
		Stickers:     stickers,
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

	// Ensure stickers directory
	stickersDir := filepath.Join(h.BaseDir, "static", "stickers")
	if err := os.MkdirAll(stickersDir, 0755); err != nil {
		h.stickerError(c, "创建目录失败")
		return
	}

	// Generate unique filename
	savedName := uuid.New().String() + ext
	dstPath := filepath.Join(stickersDir, savedName)
	dst, err := os.Create(dstPath)
	if err != nil {
		h.stickerError(c, "创建文件失败")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		h.stickerError(c, "写入文件失败")
		return
	}

	url := "/static/stickers/" + savedName
	mimeType := models.MIMEType(ext)
	if _, err := h.StickerModel.Create(savedName, url, header.Size, mimeType); err != nil {
		log.Printf("AdminCreateSticker: failed to record sticker %s: %v", savedName, err)
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
	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
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
		log.Printf("AdminDeleteSticker: sticker %d not found: %v", id, err)
		c.Redirect(http.StatusFound, "/admin/stickers")
		return
	}

	// 删除磁盘文件
	filePath := filepath.Join(h.BaseDir, "static", "stickers", sticker.Filename)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("AdminDeleteSticker: failed to remove file %s: %v", filePath, err)
	}

	// 删除数据库记录
	if err := h.StickerModel.Delete(id); err != nil {
		log.Printf("AdminDeleteSticker: failed to delete sticker %d: %v", id, err)
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

	// Generate unique filename
	savedName := uuid.New().String() + ext
	// Determine uploads dir relative to working directory
	uploadsDir := "static/uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建上传目录失败"})
		return
	}

	dstPath := filepath.Join(uploadsDir, savedName)
	dst, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文件失败"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	url := "/static/uploads/" + savedName

	// 记录到资源表（post_id 暂时为空，保存文章时关联）
	mimeType := models.MIMEType(ext)
	if _, err := h.ResourceModel.Create(savedName, url, header.Size, mimeType); err != nil {
		log.Printf("AdminUpload: failed to record resource %s: %v", savedName, err)
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ---- Helpers ----

func (h *Handler) adminDashboardWithSuccess(c *gin.Context, msg string) {
	posts, _ := h.PostModel.ListAll()
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Success:    msg,
	})
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
