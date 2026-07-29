package middleware

import (
	"fmt"

	"ofo/config"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders 注入常见安全响应头，防止 XSS / 点击劫持 / MIME 嗅探。
// CDN 域名（如有配置）会加入 CSP 的 img-src / media-src 白名单。
func SecurityHeaders(cfg *config.Config) gin.HandlerFunc {
	cdnHost := ""
	if cfg.QiniuDomain != "" {
		cdnHost = " " + cfg.QiniuDomain
	}

	csp := fmt.Sprintf(
		"default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:%s; media-src 'self' blob:%s; font-src 'self'; connect-src 'self'; frame-ancestors 'none';",
		cdnHost, cdnHost,
	)

	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", csp)
		c.Next()
	}
}
