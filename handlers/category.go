package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Category(c *gin.Context) {
	slug := c.Param("slug")

	// 一次加载最多 50 篇文章，前端 JS 控制每 15 条展示
	posts, total, err := h.PostModel.ListByCategory(slug, 0, 50)
	if err != nil || total == 0 {
		c.HTML(http.StatusNotFound, "home.html", PageData{
			Title: "404 — Category Not Found", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	// Find the category name
	categoryName := slug
	for _, cat := range categories {
		if cat.Slug == slug {
			categoryName = cat.Name
			break
		}
	}

	c.HTML(http.StatusOK, "home.html", PageData{
		Title:        "Category: " + categoryName + " — " + h.Cfg.Title,
		Description:  "Posts in category " + categoryName,
		Keywords:     categoryName + "," + h.Cfg.Keywords,
		CanonicalURL: h.Cfg.BaseURL + "/category/" + slug,
		Cfg:          h.Cfg,
		Categories:   categories,
		Tags:         tags,
		Posts:        posts,
		FilterName:   categoryName,
		CurrentPath:  "/category/" + slug,
	})
}
