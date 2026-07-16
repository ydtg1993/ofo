package handlers

import (
	"ofo/config"
	"ofo/models"
	"ofo/storage"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	PostModel     *models.PostModel
	ResourceModel *models.ResourceModel
	StickerModel  *models.StickerModel
	Cfg           *config.Config
	BaseDir       string
	Storage       storage.Storage
}

// PageData is the top-level data passed to every template.
type PageData struct {
	Title       string
	Description string
	Cfg         *config.Config
	Categories  []models.Category
	Tags        []models.Tag
	CurrentPath string
	// Content is set per-page
	Posts      []models.PostCard
	Post       *models.Post
	Pagination *models.Pagination
	Category   *models.Category
	Tag        *models.Tag
	IsAbout    bool
	Is404      bool
	// Post detail enrichment
	PostCategoryName string
	PostCategorySlug string
	PostTags         []models.Tag
	// Category/Tag filter name
	FilterName string
	// SEO fields
	Keywords     string
	CanonicalURL string
	OGImage      string
	IsHome       bool
}
