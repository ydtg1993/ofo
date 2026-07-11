package handlers

import (
	"math"
	"net/http"
	"strconv"

	"ofo/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Category(c *gin.Context) {
	slug := c.Param("slug")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 6
	offset := (page - 1) * perPage

	posts, total, err := h.PostModel.ListByCategory(slug, offset, perPage)
	if err != nil {
		c.HTML(http.StatusNotFound, "layout.html", PageData{
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
		Title:       "Category: " + categoryName + " — " + h.Cfg.Title,
		Description: "Posts in category " + categoryName,
		Cfg:         h.Cfg,
		Categories:  categories,
		Tags:        tags,
		Posts:       posts,
		Pagination:  pagination,
		FilterName:  categoryName,
		CurrentPath: "/category/" + slug,
	})
}
