package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RobotsTXT serves a robots.txt that allows all crawlers and points to the sitemap.
func (h *Handler) RobotsTXT(c *gin.Context) {
	txt := fmt.Sprintf(`User-agent: *
Allow: /
Sitemap: %s/sitemap.xml
`, h.Cfg.BaseURL)
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, txt)
}

// SitemapXML generates a dynamic XML sitemap for all published content.
func (h *Handler) SitemapXML(c *gin.Context) {
	posts, _, _ := h.PostModel.ListPublished(0, 10000)
	categories, _ := h.PostModel.AllCategories()
	tags, _ := h.PostModel.AllTags()

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
        xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">
`

	// Homepage — top priority
	xml += urlTag(h.Cfg.BaseURL, "daily", "1.0", time.Now())

	// About page
	xml += urlTag(h.Cfg.BaseURL+"/about", "monthly", "0.5", time.Now())

	// Category pages
	for _, cat := range categories {
		if cat.Count > 0 {
			xml += urlTag(h.Cfg.BaseURL+"/category/"+cat.Slug, "weekly", "0.6", time.Now())
		}
	}

	// Tag pages
	for _, tag := range tags {
		if tag.Count > 0 {
			xml += urlTag(h.Cfg.BaseURL+"/tag/"+tag.Slug, "weekly", "0.4", time.Now())
		}
	}

	// Published posts
	for _, p := range posts {
		loc := fmt.Sprintf("%s/post/%s", h.Cfg.BaseURL, p.Slug)
		lastmod := p.CreatedAt.Format("2006-01-02")
		if p.ThumbnailURL != "" {
			xml += fmt.Sprintf(`  <url>
    <loc>%s</loc>
    <lastmod>%s</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.8</priority>
    <image:image>
      <image:loc>%s</image:loc>
    </image:image>
  </url>
`, loc, lastmod, p.ThumbnailURL)
		} else {
			xml += urlTag(loc, "monthly", "0.8", p.CreatedAt)
		}
	}

	xml += `</urlset>`

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.String(http.StatusOK, xml)
}

func urlTag(loc, changefreq, priority string, lastmod time.Time) string {
	return fmt.Sprintf(`  <url>
    <loc>%s</loc>
    <lastmod>%s</lastmod>
    <changefreq>%s</changefreq>
    <priority>%s</priority>
  </url>
`, loc, lastmod.Format("2006-01-02"), changefreq, priority)
}
