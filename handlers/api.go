package handlers

import (
	"net/http"
	"strconv"

	"ofo/logger"
	"ofo/models"

	"github.com/gin-gonic/gin"
)

// APIPosts returns a paginated JSON list of published posts.
// Supports optional ?category=slug filter.
func (h *Handler) APIPosts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "15"))
	categorySlug := c.Query("category")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 15
	}

	offset := (page - 1) * perPage

	var posts []models.PostCard
	var total int
	var err error

	if categorySlug != "" {
		posts, total, err = h.PostModel.ListByCategory(categorySlug, offset, perPage)
	} else {
		posts, total, err = h.PostModel.ListPublished(offset, perPage)
	}

	if err != nil {
		logger.ErrorWithContext(c, "failed to list posts for API", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if posts == nil {
		posts = []models.PostCard{}
	}

	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"posts":       posts,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// APIPost returns a single post as JSON with full content.
func (h *Handler) APIPost(c *gin.Context) {
	slug := c.Param("slug")
	post, err := h.PostModel.GetBySlug(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		return
	}

	tags, _ := h.PostModel.TagsForPost(post.ID)
	categoryName := h.PostModel.GetCategoryName(post.CategoryID)
	categorySlug := h.PostModel.GetCategorySlug(post.CategoryID)

	c.JSON(http.StatusOK, gin.H{
		"id":            post.ID,
		"title":         post.Title,
		"slug":          post.Slug,
		"excerpt":       post.Excerpt,
		"content_html":  post.ContentHTML,
		"thumbnail_url": post.ThumbnailURL,
		"category_name": categoryName,
		"category_slug": categorySlug,
		"created_at":    post.CreatedAt,
		"updated_at":    post.UpdatedAt,
		"tags":          tags,
	})
}
