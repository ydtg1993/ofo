package handlers

import (
	"net/http"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Tag(c *gin.Context) {
	slug := c.Param("slug")

	// 一次加载最多 50 篇文章，前端 JS 控制每 15 条展示
	posts, total, err := h.PostModel.ListByTag(slug, 0, 50)
	if err != nil || total == 0 {
		if err != nil {
			logger.WarnWithContext(c, "failed to list posts by tag", "slug", slug, "err", err)
		}
		c.HTML(http.StatusNotFound, "home.html", PageData{
			Title: "404 — Tag Not Found", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, err := h.PostModel.AllCategories()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories", "err", err)
	}
	tags, err := h.PostModel.AllTags()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags", "err", err)
	}

	// Find the tag name
	tagName := slug
	for _, tag := range tags {
		if tag.Slug == slug {
			tagName = tag.Name
			break
		}
	}

	pd := PageData{
		Title:         "Tag: " + tagName + " — " + h.Cfg.Title,
		Description:   "Posts tagged with " + tagName,
		Keywords:      tagName + "," + h.Cfg.Keywords,
		CanonicalURL:  h.Cfg.BaseURL + "/tag/" + slug,
		Cfg:           h.Cfg,
		Categories:    categories,
		Tags:          tags,
		Posts:         posts,
		FilterName:    tagName,
		FishModeTitle: h.Cfg.FishModeTitle,
		CurrentPath:   "/tag/" + slug,
	}

	tmpl := "home.html"
	if isMobileDevice(c.GetHeader("User-Agent")) {
		tmpl = "mobile_home"
	}
	c.HTML(http.StatusOK, tmpl, pd)
}
