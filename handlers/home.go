package handlers

import (
	"math"
	"net/http"
	"strconv"

	"ofo/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Home(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 6
	offset := (page - 1) * perPage

	posts, total, err := h.PostModel.ListPublished(offset, perPage)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "layout.html", PageData{
			Title: "Error", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	pagination := &models.Pagination{
		CurrentPage: page,
		TotalPages:  totalPages,
		PerPage:     perPage,
		TotalPosts:  total,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}

	c.HTML(http.StatusOK, "layout.html", PageData{
		Title:       h.Cfg.Title,
		Description: "搞笑图片、趣味短片、奇闻趣事 —— 内容来源于网络，快乐来源于分享。",
		Cfg:         h.Cfg,
		Categories:  categories,
		Tags:        tags,
		Posts:       posts,
		Pagination:  pagination,
		CurrentPath: "/",
	})
}
