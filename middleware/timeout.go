package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/timeout"
	"github.com/gin-gonic/gin"
)

// Timeout 请求超时中间件。
// d 为超时时长，超时后返回 504 Gateway Timeout。
// 注意：gin-contrib/timeout 与静态文件服务存在兼容性问题，
// 因此对 /static/ 路径跳过超时控制。
func Timeout(d time.Duration) gin.HandlerFunc {
	timeoutHandler := timeout.New(
		timeout.WithTimeout(d),
		timeout.WithHandler(func(c *gin.Context) {
			c.Next()
		}),
		timeout.WithResponse(func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
				"error": "request timeout",
			})
		}),
	)

	return func(c *gin.Context) {
		// 静态资源跳过超时控制，避免 gin-contrib/timeout 状态码传递 bug
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}
		timeoutHandler(c)
	}
}
