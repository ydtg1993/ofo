package router

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"ofo/config"
	"ofo/handlers"
	"ofo/logger"
	"ofo/middleware"

	"github.com/gin-gonic/gin"
)

// swJSContent is loaded from static/js/sw.js at startup.
var swJSContent string

// Setup 配置并返回完整的 Gin 引擎。
// 包含：模板函数、中间件链、静态资源、公开路由、管理后台路由、404 处理。
// baseDir: 项目根目录的绝对路径，用于解析模板和静态资源。
func Setup(cfg *config.Config, h *handlers.Handler, baseDir string) *gin.Engine {
	// Load Service Worker file
	if data, err := os.ReadFile(filepath.Join(baseDir, "static", "js", "sw.js")); err == nil {
		swJSContent = string(data)
	}
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// ==========================================
	// 全局中间件链（按执行顺序排列）
	// ==========================================
	r.Use(
		middleware.RequestID(),                // 1. UUID 注入
		gin.Recovery(),                        // 2. Panic 恢复
		middleware.Logger(),                   // 3. 请求日志
		middleware.Timeout(60*time.Second),    // 4. 超时控制
		middleware.SecurityHeaders(cfg),       // 5. 安全响应头
		middleware.CORS(),                     // 6. 跨域支持
		middleware.RateLimit(50, time.Second), // 7. IP 限流
	)

	// ==========================================
	// 模板引擎配置
	// ==========================================
	r.SetFuncMap(templateFuncMap(cfg, h.Storage, h.VideoSegmentModel))

	tmplDir := filepath.Join(baseDir, "templates")
	var tmplFiles []string
	filepath.Walk(tmplDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error("failed to walk template directory", "path", path, "err", err)
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".html") {
			tmplFiles = append(tmplFiles, path)
		}
		return nil
	})
	r.LoadHTMLFiles(tmplFiles...)

	// ==========================================
	// 静态资源（CSS / JS / 图片）
	// ==========================================
	// CSS / JS / 资源 — 长期缓存（URL 有版本号 ?v=，更新即失效）
	{
		cached := r.Group("/static/css")
		cached.Use(middleware.CacheControl(365 * 24 * time.Hour))
		cached.Static("", filepath.Join(baseDir, "static", "css"))
	}
	{
		cached := r.Group("/static/js")
		cached.Use(middleware.CacheControl(365 * 24 * time.Hour))
		cached.Static("", filepath.Join(baseDir, "static", "js"))
	}
	{
		cached := r.Group("/static/resources")
		cached.Use(middleware.CacheControl(365 * 24 * time.Hour))
		cached.Static("", filepath.Join(baseDir, "static", "resources"))
	}

	// 受保护的静态资源（用户上传）— 中期缓存 + 防盗链
	// 仅在本地存储模式下启用；七牛云模式下文件由 CDN 提供
	if h.Storage.IsLocal() {
		uploadsGroup := r.Group("/static/uploads")
		uploadsGroup.Use(middleware.CacheControl(7 * 24 * time.Hour))
		if cfg.StaticRateLimit > 0 {
			uploadsGroup.Use(middleware.RateLimit(cfg.StaticRateLimit, time.Second))
		}
		uploadsGroup.Use(middleware.HotlinkProtection(cfg))
		uploadsGroup.Static("", filepath.Join(baseDir, "static", "uploads"))
	}

	r.GET("/favicon.ico", func(c *gin.Context) { c.Status(204) })

	// ==========================================
	// 路由注册（公开 + 后台 + API + 404）
	// ==========================================
	registerPublicRoutes(r, cfg, h)

	return r
}
