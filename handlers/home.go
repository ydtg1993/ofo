package handlers

import (
	"net/http"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Home(c *gin.Context) {
	// 首次加载 15 篇文章，后续通过 AJAX 无限滚动加载
	posts, total, err := h.PostModel.ListPublished(0, 15)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list published posts", "err", err)
		c.HTML(http.StatusInternalServerError, "home.html", PageData{
			Title: "Error", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, err := h.PostModel.AllCategories()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for home", "err", err)
	}
	tags, err := h.PostModel.AllTags()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for home", "err", err)
	}

	c.HTML(http.StatusOK, "home.html", PageData{
		Title:         h.Cfg.Title,
		Description:   "摸鱼日报 — 30秒速览、3分钟摸鱼、午休放松。为打工人量身定制的办公室轻娱乐内容。",
		Keywords:      h.Cfg.Keywords,
		CanonicalURL:  h.Cfg.BaseURL,
		Cfg:           h.Cfg,
		Categories:    categories,
		Tags:          tags,
		Posts:         posts,
		TotalPosts:    total,
		HasMore:       total > 15,
		FishModeTitle: h.Cfg.FishModeTitle,
		CurrentPath:   "/",
		IsHome:        true,
	})
}
