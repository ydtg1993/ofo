package handlers

import (
	"net/http"
	"strings"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Post(c *gin.Context) {
	slug := c.Param("slug")

	post, err := h.PostModel.GetBySlug(slug)
	if err != nil {
		logger.WarnWithContext(c, "post not found", "slug", slug, "err", err)
		c.HTML(http.StatusNotFound, "post.html", PageData{
			Title: "404 — 文章未找到", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	tags, err := h.PostModel.TagsForPost(post.ID)
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for post", "postID", post.ID, "err", err)
	}
	categories, err := h.PostModel.AllCategories()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for post", "err", err)
	}
	allTags, err := h.PostModel.AllTags()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load all tags for post", "err", err)
	}

	categoryName := h.PostModel.GetCategoryName(post.CategoryID)
	categorySlug := h.PostModel.GetCategorySlug(post.CategoryID)

	// —— 构建 SEO keywords ——
	// 标题 + 分类名 + 标签 + 站点默认关键词，去重
	seen := make(map[string]bool)
	var kwParts []string
	addKW := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			kwParts = append(kwParts, s)
		}
	}
	// 文章标题作为最重要的关键词
	addKW(post.Title)
	// 分类名
	if categoryName != "" {
		addKW(categoryName)
	}
	// 标签
	for _, t := range tags {
		addKW(t.Name)
	}
	// 站点默认关键词
	for _, kw := range strings.Split(h.Cfg.Keywords, ",") {
		addKW(strings.TrimSpace(kw))
	}
	keywords := strings.Join(kwParts, ",")

	// OG image from thumbnail
	ogImage := ""
	if post.ThumbnailURL != "" {
		ogImage = post.ThumbnailURL
	}

	// 根据分类计算阅读时长
	readTime := readTimeForCategory(categorySlug)

	// 上一篇 / 下一篇导航
	prevPost, nextPost, _ := h.PostModel.GetAdjacentPosts(slug)

	c.HTML(http.StatusOK, "post.html", PageData{
		Title:            post.Title + " — " + h.Cfg.Title,
		Description:      post.Excerpt,
		Keywords:         keywords,
		CanonicalURL:     h.Cfg.BaseURL + "/post/" + slug,
		OGImage:          ogImage,
		Cfg:              h.Cfg,
		Categories:       categories,
		Tags:             allTags,
		Post:             post,
		PostCategoryName: categoryName,
		PostCategorySlug: categorySlug,
		PostTags:         tags,
		ReadTime:         readTime,
		PrevPost:         prevPost,
		NextPost:         nextPost,
		FishModeTitle:    h.Cfg.FishModeTitle,
		CurrentPath:      "/post/" + slug,
	})
}

// readTimeForCategory 根据分类 slug 计算估算阅读时长。
func readTimeForCategory(categorySlug string) string {
	switch categorySlug {
	case "quick-peek":
		return "30秒"
	case "bathroom-break":
		return "3-5分钟"
	case "lunch-break":
		return "10-15分钟"
	case "daily-highlight":
		return "5-10分钟"
	default:
		return "约5分钟"
	}
}
