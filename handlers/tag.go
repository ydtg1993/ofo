package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Tag(c *gin.Context) {
	slug := c.Param("slug")

	// 一次加载最多 50 篇文章，前端 JS 控制每 15 条展示
	posts, total, err := h.PostModel.ListByTag(slug, 0, 50)
	if err != nil || total == 0 {
		c.HTML(http.StatusNotFound, "home.html", PageData{
			Title: "404 — Tag Not Found", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	// Find the tag name
	tagName := slug
	for _, tag := range tags {
		if tag.Slug == slug {
			tagName = tag.Name
			break
		}
	}

	c.HTML(http.StatusOK, "home.html", PageData{
		Title:        "Tag: " + tagName + " — " + h.Cfg.Title,
		Description:  "Posts tagged with " + tagName,
		Keywords:     tagName + "," + h.Cfg.Keywords,
		CanonicalURL: h.Cfg.BaseURL + "/tag/" + slug,
		Cfg:          h.Cfg,
		Categories:   categories,
		Tags:         tags,
		Posts:        posts,
		FilterName:   tagName,
		CurrentPath:  "/tag/" + slug,
	})
}
