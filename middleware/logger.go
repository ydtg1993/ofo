package middleware

import (
	"time"

	"ofo/logger"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// Logger records the request method, path, status code, latency, and client IP.
// Log level is determined by HTTP status:
//   - 5xx → Error
//   - 4xx → Warn
//   - other → Info
//
// Requires RequestID middleware to be registered first.
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

		attrs := []any{
			"req_id", reqID,
			"status", statusCode,
			"latency", latency,
			"ip", clientIP,
			"method", method,
			"path", path,
		}

		switch {
		case statusCode >= 500:
			logger.Error("request", attrs...)
		case statusCode >= 400:
			logger.Warn("request", attrs...)
		default:
			logger.Info("request", attrs...)
		}
	}
}
