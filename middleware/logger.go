package middleware

import (
	"log"
	"time"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// Logger 记录请求方法、路径、状态码、耗时、客户端 IP。
// 依赖 requestid 中间件提供的请求 ID。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()
		reqID := requestid.Get(c)

		if query != "" {
			path = path + "?" + query
		}

		log.Printf("[%s] | %3d | %12v | %-15s | %-7s | %s",
			reqID, statusCode, latency, clientIP, method, path,
		)
	}
}
