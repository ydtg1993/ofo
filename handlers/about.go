package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) About(c *gin.Context) {
	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	pd := PageData{
		Title:         "关于 — " + h.Cfg.Title,
		Description:   "关于蹬车摸鱼和作者",
		Keywords:      h.Cfg.Keywords,
		CanonicalURL:  h.Cfg.BaseURL + "/about",
		Cfg:           h.Cfg,
		Categories:    categories,
		Tags:          tags,
		IsAbout:       true,
		FishModeTitle: h.Cfg.FishModeTitle,
		CurrentPath:   "/about",
	}

	tmpl := "about.html"
	if isMobileDevice(c.GetHeader("User-Agent")) {
		tmpl = "mobile_about"
	}
	c.HTML(http.StatusOK, tmpl, pd)
}
