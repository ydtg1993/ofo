package admin

import (
	"net/http"
	"regexp"
	"strings"

	"ofo/config"
	"ofo/handlers"
	"ofo/models"

	"github.com/gin-gonic/gin"
)

// ---- Editor Preview URL Resolution ----

// reUploadURL matches /uploads/... URLs (with optional subdirectories like year/month)
// in markdown or HTML content.
var reUploadURL = regexp.MustCompile(`/uploads/(?:[^"'<>\s]+/)*[^"'<>\s]+`)

// ResolveContentURLs rewrites /uploads/... URLs in markdown/HTML content to display
// URLs by looking up each URL in the resources table. Uses per-resource storage
// backend (local/qiniu) to construct the correct preview URL.
// Falls back to the global config if a resource record is not found.
func ResolveContentURLs(content string, rm *models.ResourceModel, cfg *config.Config) string {
	return reUploadURL.ReplaceAllStringFunc(content, func(match string) string {
		r, err := rm.FindByURL(match)
		if err != nil {
			// Resource not tracked — fall back to config default
			return handlers.DisplayURL(match, cfg)
		}
		// Rewrite based on the resource's actual storage backend
		if r.Storage == "qiniu" && cfg.QiniuDomain != "" {
			return strings.TrimRight(cfg.QiniuDomain, "/") + r.URL
		}
		// Local (or unknown storage): /static/uploads/...
		return "/static" + r.URL
	})
}

// AdminResolveContent is the AJAX endpoint for editor preview URL resolution.
// POST /admin/resolve-content
// Request:  {"content": "markdown or HTML..."}
// Response: {"content": "rewritten content with display URLs"}
func (a *AdminHandler) AdminResolveContent(c *gin.Context) {
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	resolved := ResolveContentURLs(body.Content, a.ResourceModel, a.Cfg)
	c.JSON(http.StatusOK, gin.H{"content": resolved})
}
