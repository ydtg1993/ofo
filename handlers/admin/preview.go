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

// reUploadURL matches /uploads/... URLs — both relative and absolute (CDN-prefixed).
var reUploadURL = regexp.MustCompile(`(?:https?://[^/"'\s]+)?/uploads/(?:[^"'<>\s]+/)*[^"'<>\s]+`)

// ResolveContentURLs rewrites /uploads/... URLs in markdown/HTML content to display
// URLs by looking up each URL in the resources table. Uses per-resource storage
// backend (local/qiniu) to construct the correct preview URL.
// CDN-prefixed URLs are normalized first to avoid double-prefixing.
func ResolveContentURLs(content string, rm *models.ResourceModel, cfg *config.Config) string {
	return reUploadURL.ReplaceAllStringFunc(content, func(match string) string {
		// Normalize: strip CDN domain prefix if present
		rel := match
		if cfg.QiniuDomain != "" {
			domain := strings.TrimRight(cfg.QiniuDomain, "/")
			rel = strings.TrimPrefix(rel, domain)
		}
		r, err := rm.FindByURL(rel)
		if err != nil {
			return handlers.DisplayURL(rel, cfg)
		}
		if r.Storage == "qiniu" && cfg.QiniuDomain != "" {
			return strings.TrimRight(cfg.QiniuDomain, "/") + r.URL
		}
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
