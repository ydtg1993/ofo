package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) About(c *gin.Context) {
	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	c.HTML(http.StatusOK, "layout.html", PageData{
		Title:       "关于 — " + h.Cfg.Title,
		Description: "关于本站和作者",
		Cfg:         h.Cfg,
		Categories:  categories,
		Tags:        tags,
		IsAbout:     true,
		CurrentPath: "/about",
	})
}
