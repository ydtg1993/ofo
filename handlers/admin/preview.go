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
// Response: {"content": "full processed HTML matching frontend rendering"}
// Applies the same pipeline as the lazyImages template function:
// markdown→HTML → URL rewrite → lazy loading → image/video dimensions →
// deferred video src → video poster injection.
func (a *AdminHandler) AdminResolveContent(c *gin.Context) {
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// 1. Convert markdown to HTML (includes InjectLazyLoading)
	html := renderMarkdown(body.Content)

	// 2. Rewrite /uploads/ URLs to display URLs (CDN or local /static prefix)
	if !a.Cfg.MediaProtection {
		html = handlers.RewriteContentURLs(html, a.Cfg)
	}

	// 3. Inject image dimensions (width/height + aspect-ratio for CLS prevention)
	html = InjectImageDimensions(html, a.Storage)

	// 4. Inject video dimensions
	html = InjectVideoDimensions(html, a.Storage)

	// 5. Defer video src → data-src (prevents eager video loading in preview)
	html = DeferVideoSrc(html)

	// 6. Inject poster from video_segments table
	html = InjectVideoPoster(html, a.VideoSegmentModel, func(p string) string {
		return handlers.DisplayURL(p, a.Cfg)
	})

	c.JSON(http.StatusOK, gin.H{"content": html})
}
