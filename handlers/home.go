package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Home(c *gin.Context) {
	// 一次加载最多 50 篇文章，前端 JS 控制每 10 条展示
	posts, _, err := h.PostModel.ListPublished(0, 50)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "layout.html", PageData{
			Title: "Error", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	c.HTML(http.StatusOK, "layout.html", PageData{
		Title:       h.Cfg.Title,
		Description: "搞笑图片、趣味短片、奇闻趣事 —— 内容来源于网络，快乐来源于分享。",
		Cfg:         h.Cfg,
		Categories:  categories,
		Tags:        tags,
		Posts:       posts,
		CurrentPath: "/",
	})
}
