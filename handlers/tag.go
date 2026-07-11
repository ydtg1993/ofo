package handlers

import (
	"math"
	"net/http"
	"strconv"

	"ofo/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Tag(c *gin.Context) {
	slug := c.Param("slug")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 6
	offset := (page - 1) * perPage

	posts, total, err := h.PostModel.ListByTag(slug, offset, perPage)
	if err != nil {
		c.HTML(http.StatusNotFound, "layout.html", PageData{
			Title: "404 — Tag Not Found", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	// Find the tag name
	tagName := slug
	for _, t := range tags {
		if t.Slug == slug {
			tagName = t.Name
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
		Title:       "Tag: " + tagName + " — " + h.Cfg.Title,
		Description: "Posts tagged with " + tagName,
		Cfg:         h.Cfg,
		Categories:  categories,
		Tags:        tags,
		Posts:       posts,
		Pagination:  pagination,
		FilterName:  tagName,
		CurrentPath: "/tag/" + slug,
	})
}
