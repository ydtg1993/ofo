package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) About(c *gin.Context) {
	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	c.HTML(http.StatusOK, "about.html", PageData{
		Title:         "关于 — " + h.Cfg.Title,
		Description:   "关于摸鱼日报和作者",
		Keywords:      h.Cfg.Keywords,
		CanonicalURL:  h.Cfg.BaseURL + "/about",
		Cfg:           h.Cfg,
		Categories:    categories,
		Tags:          tags,
		IsAbout:       true,
		FishModeTitle: h.Cfg.FishModeTitle,
		CurrentPath:   "/about",
	})
}
