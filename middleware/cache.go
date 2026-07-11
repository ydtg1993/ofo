package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// CacheControl 设置 Cache-Control 响应头。
// maxAge > 0 时启用缓存；maxAge <= 0 时为 no-cache。
func CacheControl(maxAge time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxAge > 0 {
			c.Header("Cache-Control", fmt.Sprintf("public, max-age=%.0f", maxAge.Seconds()))
		} else {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		c.Next()
	}
}
