package router

import (
	"net/http"
	"strings"
	"time"

	"ofo/config"
	"ofo/handlers"
	"ofo/handlers/admin"
	"ofo/middleware"

	"github.com/gin-gonic/gin"
)

// registerPublicRoutes sets up all public-facing routes, admin routes, API, and 404.
func registerPublicRoutes(r *gin.Engine, cfg *config.Config, h *handlers.Handler) {
	// ==========================================
	// Service Worker（拦截 blob: 导航）
	// ==========================================
	r.GET("/sw.js", func(c *gin.Context) {
		c.Header("Service-Worker-Allowed", "/")
		c.Data(200, "application/javascript; charset=utf-8", []byte(swJSContent))
	})

	// ==========================================
	// 媒体代理路由（Blob 方式加载，防止爬取）
	// ==========================================
	if cfg.MediaProtection {
		mediaGroup := r.Group("/media")
		if cfg.StaticRateLimit > 0 {
			mediaGroup.Use(middleware.RateLimit(cfg.StaticRateLimit, time.Second))
		}
		mediaGroup.GET("/*filepath", h.MediaProxy)
	}

	// ==========================================
	// 公开路由
	// ==========================================
	r.GET("/", h.Home)                   // 首页（分页文章列表）
	r.GET("/post/:slug", h.Post)         // 文章详情
	r.GET("/category/:slug", h.Category) // 分类筛选
	r.GET("/tag/:slug", h.Tag)           // 标签筛选
	r.GET("/fullscreen", h.Fullscreen)   // 全屏刷屏模式
	r.GET("/about", h.About)             // 关于页面
	r.GET("/rss.xml", h.RSS)             // RSS 订阅
	r.GET("/feed.xml", h.RSS)            // RSS 别名
	r.GET("/robots.txt", h.RobotsTXT)    // 搜索引擎爬虫规则
	r.GET("/sitemap.xml", h.SitemapXML)  // 站点地图

	// ==========================================
	// API（为桌面客户端准备）
	// ==========================================
	api := r.Group("/api")
	api.Use(middleware.CacheControl(5 * time.Minute))
	{
		api.GET("/posts", h.APIPosts)
		api.GET("/posts/:slug", h.APIPost)
	}

	r.GET("/verification.html", func(c *gin.Context) {
		c.HTML(http.StatusOK, "verification.html", gin.H{})
	})

	// ==========================================
	// 管理后台路由 (/admin)
	// ==========================================
	admin.SetupRoutes(r, h)

	// ==========================================
	// 404 兜底
	// ==========================================
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Status(404)
			return
		}
		c.HTML(404, "404.html", handlers.PageData{
			Title:        "404 — 页面未找到",
			Description:  "页面未找到",
			Keywords:     cfg.Keywords,
			CanonicalURL: cfg.BaseURL + c.Request.URL.Path,
			Cfg:          cfg,
			Is404:        true,
		})
	})
}
