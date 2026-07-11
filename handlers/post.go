package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Post(c *gin.Context) {
	slug := c.Param("slug")

	post, err := h.PostModel.GetBySlug(slug)
	if err != nil {
		c.HTML(http.StatusNotFound, "layout.html", PageData{
			Title: "404 — 文章未找到", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	tags, _ := h.PostModel.TagsForPost(post.ID)
	categories, _ := h.PostModel.AllCategories()
	allTags, _ := h.PostModel.AllTags()

	categoryName := h.PostModel.GetCategoryName(post.CategoryID)
	categorySlug := h.PostModel.GetCategorySlug(post.CategoryID)

	c.HTML(http.StatusOK, "layout.html", PageData{
		Title:            post.Title + " — " + h.Cfg.Title,
		Description:      post.Excerpt,
		Cfg:              h.Cfg,
		Categories:       categories,
		Tags:             allTags,
		Post:             post,
		PostCategoryName: categoryName,
		PostCategorySlug: categorySlug,
		PostTags:         tags,
		CurrentPath:      "/post/" + slug,
	})
}
