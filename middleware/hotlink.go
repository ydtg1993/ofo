package middleware

import (
	"net/url"
	"strings"

	"ofo/config"

	"github.com/gin-gonic/gin"
)

// mediaExtensions are file extensions that should be protected from hotlinking.
var mediaExtensions = map[string]bool{
	// 图片
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".svg": true, ".bmp": true, ".ico": true,
	// 视频
	".mp4": true, ".webm": true, ".mov": true, ".avi": true, ".ogg": true,
}

// searchEngineDomains contains known crawler / search-engine domains that are
// allowed to fetch media (so thumbnails still appear in image search results).
var searchEngineDomains = []string{
	"google.com", "googlebot.com",
	"bing.com", "bingbot.com",
	"baidu.com", "baidu.jp",
	"yandex.com", "yandex.net",
	"duckduckgo.com",
	"sogou.com",
	"360.cn", "so.com",
	"yahoo.com",
}

// HotlinkProtection returns a middleware that blocks requests to image/video
// files when the Referer header indicates the request is embedded on an
// external site (hotlinking / 盗链).
//
// Rules:
//   - Empty/missing Referer → allow (direct browser access, RSS readers, etc.)
//   - Referer matches own site domain → allow
//   - Referer is a known search engine → allow (image search indexing)
//   - Referer matches user-configured allowed list → allow
//   - Everything else → 403 Forbidden
//
// The middleware is a no-op when cfg.HotlinkProtection is false.
func HotlinkProtection(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 功能开关
		if !cfg.HotlinkProtection {
			c.Next()
			return
		}

		// 只保护媒体资源（按扩展名判断）
		path := c.Request.URL.Path
		ext := ""
		if dot := strings.LastIndex(path, "."); dot >= 0 {
			ext = strings.ToLower(path[dot:])
		}
		if !mediaExtensions[ext] {
			c.Next()
			return
		}

		referer := c.Request.Referer()

		// 规则 1: 空 Referer（直接访问、书签、RSS 阅读器）→ 放行
		if referer == "" {
			c.Next()
			return
		}

		refererURL, err := url.Parse(referer)
		if err != nil {
			// 无法解析的 Referer → 拒绝
			c.AbortWithStatus(403)
			return
		}

		refererHost := strings.ToLower(refererURL.Hostname())

		// 规则 2: 本站域名 → 放行
		if isOwnDomain(refererHost, cfg) {
			c.Next()
			return
		}

		// 规则 3: 搜索引擎爬虫 → 放行
		if isSearchEngine(refererHost) {
			c.Next()
			return
		}

		// 规则 4: 用户配置的额外允许域名 → 放行
		for _, allowed := range cfg.AllowedReferrers {
			if strings.Contains(refererHost, strings.ToLower(strings.TrimSpace(allowed))) {
				c.Next()
				return
			}
		}

		// 默认：拒绝盗链
		c.AbortWithStatus(403)
	}
}

// isOwnDomain checks whether the referer host equals the site's own domain
// (or a subdomain of it).
func isOwnDomain(refererHost string, cfg *config.Config) bool {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return false
	}
	ownHost := strings.ToLower(baseURL.Hostname())

	// 精确匹配
	if refererHost == ownHost {
		return true
	}
	// 子域名匹配（如 www.example.com 匹配 example.com）
	if strings.HasSuffix(refererHost, "."+ownHost) {
		return true
	}
	return false
}

// isSearchEngine returns true when the host appears to belong to a known
// search engine or crawler.
func isSearchEngine(host string) bool {
	for _, se := range searchEngineDomains {
		if strings.Contains(host, se) {
			return true
		}
	}
	return false
}
