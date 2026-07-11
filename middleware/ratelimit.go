package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	limiterGin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	limiterMemory "github.com/ulule/limiter/v3/drivers/store/memory"
)

// RateLimit IP 限流中间件（基于 ulule/limiter 的令牌桶算法）。
// requests: 时间窗口内允许的请求数
// per: 每周期时长
func RateLimit(requests int, per time.Duration) gin.HandlerFunc {
	rate := limiter.Rate{
		Period: per,
		Limit:  int64(requests),
	}
	store := limiterMemory.NewStore()
	instance := limiter.New(store, rate)

	return limiterGin.NewMiddleware(instance)
}
