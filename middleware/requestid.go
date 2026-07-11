package middleware

import (
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// RequestID 为每个请求附加唯一 ID（使用 gin-contrib/requestid）。
// 注入 X-Request-ID 响应头，可通过 requestid.Get(c) 在后续中间件中读取。
func RequestID() gin.HandlerFunc {
	return requestid.New(
		requestid.WithCustomHeaderStrKey("X-Request-ID"),
	)
}
