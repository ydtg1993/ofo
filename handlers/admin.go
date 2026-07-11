package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ofo/middleware"
	"ofo/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

// sanitizePolicy returns a bluemonday policy that allows video elements.
func sanitizePolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("video", "source")
	p.AllowAttrs("src", "controls", "width", "height", "autoplay", "loop", "muted", "poster").OnElements("video")
	p.AllowAttrs("src", "type").OnElements("source")
	return p
}

// renderMarkdown converts markdown to sanitized HTML.
func renderMarkdown(md string) string {
	unsafe := blackfriday.Run([]byte(md))
	return string(sanitizePolicy().SanitizeBytes(unsafe))
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
	IsEditing      bool
	IsNew          bool
	ShowCategories bool
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
		c.HTML(http.StatusInternalServerError, "admin.html", AdminPageData{
			Title: "Dashboard",
			Cfg:   h.Cfg,
			Error: "加载文章列表失败",
		})
		return
	}

	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
	})
}

// ---- New Post Form ----

func (h *Handler) AdminNewPost(c *gin.Context) {
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin.html", AdminPageData{
		Title:      "New Post",
		Cfg:        h.Cfg,
		IsNew:      true,
		Categories: categories,
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

	c.HTML(http.StatusOK, "admin.html", AdminPageData{
		Title:      "Edit: " + post.Title,
		Cfg:        h.Cfg,
		Post:       post,
		Categories: categories,
		Tags:       tags,
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
	excerpt := extractExcerptStr(contentMD, 200)

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

	_, err := h.PostModel.Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, tagNames)
	if err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin.html", AdminPageData{
			Title:      "New Post",
			Cfg:        h.Cfg,
			IsNew:      true,
			Categories: categories,
			Error:      "保存失败：" + err.Error(),
		})
		return
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
	excerpt := extractExcerptStr(contentMD, 200)

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

	if err := h.PostModel.Update(id, title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, tagNames); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		tags, _ := h.PostModel.TagsForPost(id)
		post, _ := h.PostModel.GetByID(id)
		c.HTML(http.StatusOK, "admin.html", AdminPageData{
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

	h.adminDashboardWithSuccess(c, "文章更新成功")
}

// ---- Delete Post ----

func (h *Handler) AdminDeletePost(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := h.PostModel.Delete(id); err != nil {
		h.adminDashboardWithSuccess(c, "删除文章失败")
		return
	}

	h.adminDashboardWithSuccess(c, "文章删除成功")
}

// ---- Category Management ----

func (h *Handler) AdminCategories(c *gin.Context) {
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin.html", AdminPageData{
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
		c.HTML(http.StatusOK, "admin.html", AdminPageData{
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
		c.HTML(http.StatusOK, "admin.html", AdminPageData{
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
		c.HTML(http.StatusOK, "admin.html", AdminPageData{
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
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ---- Helpers ----

func (h *Handler) adminDashboardWithSuccess(c *gin.Context, msg string) {
	posts, _ := h.PostModel.ListAll()
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Success:    msg,
	})
}

func slugifyStr(s string) string {
	result := ""
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result += string(r)
		} else if r == ' ' || r == '-' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
			}
		}
	}
	return strings.Trim(result, "-")
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
	parts := strings.Split(tagStr, ",")
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
