package handlers

import (
	"net/http"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Home(c *gin.Context) {
	// 一次加载最多 50 篇文章，前端 JS 控制每 10 条展示
	posts, _, err := h.PostModel.ListPublished(0, 50)
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
		Title:        h.Cfg.Title,
		Description:  "搞笑图片、趣味短片、奇闻趣事 —— 内容来源于网络，快乐来源于分享。",
		Keywords:     h.Cfg.Keywords,
		CanonicalURL: h.Cfg.BaseURL,
		Cfg:          h.Cfg,
		Categories:   categories,
		Tags:         tags,
		Posts:        posts,
		CurrentPath:  "/",
		IsHome:       true,
	})
}
