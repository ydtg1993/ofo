package router

import (
	"database/sql"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ofo/config"
	"ofo/handlers"
	"ofo/logger"
	"ofo/middleware"
	"ofo/models"
	"ofo/storage"

	"github.com/gin-gonic/gin"
)

// Setup 配置并返回完整的 Gin 引擎。
// 包含：模板函数、中间件链、静态资源、公开路由、管理后台路由、404 处理。
// baseDir: 项目根目录的绝对路径，用于解析模板和静态资源。
// swJSContent is loaded from static/js/sw.js at startup.
var swJSContent string

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
		middleware.Timeout(30*time.Second),    // 4. 超时控制
		middleware.SecurityHeaders(),          // 5. 安全响应头
		middleware.CORS(),                     // 6. 跨域支持
		middleware.RateLimit(50, time.Second), // 7. IP 限流
	)

	// ==========================================
	// 模板引擎配置
	// ==========================================
	r.SetFuncMap(templateFuncMap(cfg, baseDir, h.Storage))
	// 递归加载 templates/ 下所有 .html 文件
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

	// 受保护的静态资源（用户上传 / 表情包）— 中期缓存 + 防盗链
	// 仅在本地存储模式下启用；七牛云模式下文件由 CDN 提供
	if h.Storage.IsLocal() {
		uploadsGroup := r.Group("/static/uploads")
		uploadsGroup.Use(middleware.CacheControl(7 * 24 * time.Hour))
		if cfg.StaticRateLimit > 0 {
			uploadsGroup.Use(middleware.RateLimit(cfg.StaticRateLimit, time.Second))
		}
		uploadsGroup.Use(middleware.HotlinkProtection(cfg))
		uploadsGroup.Static("", filepath.Join(baseDir, "static", "uploads"))

		stickersGroup := r.Group("/static/stickers")
		stickersGroup.Use(middleware.CacheControl(7 * 24 * time.Hour))
		if cfg.StaticRateLimit > 0 {
			stickersGroup.Use(middleware.RateLimit(cfg.StaticRateLimit, time.Second))
		}
		stickersGroup.Use(middleware.HotlinkProtection(cfg))
		stickersGroup.Static("", filepath.Join(baseDir, "static", "stickers"))
	}

	r.GET("/favicon.ico", func(c *gin.Context) { c.Status(204) })

	// Service Worker（拦截 blob: 导航）
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
	{
		r.GET("/", h.Home)                   // 首页（分页文章列表）
		r.GET("/post/:slug", h.Post)         // 文章详情
		r.GET("/category/:slug", h.Category) // 分类筛选
		r.GET("/tag/:slug", h.Tag)           // 标签筛选
		r.GET("/about", h.About)             // 关于页面
		r.GET("/rss.xml", h.RSS)             // RSS 订阅
		r.GET("/feed.xml", h.RSS)            // RSS 别名
		r.GET("/robots.txt", h.RobotsTXT)    // 搜索引擎爬虫规则
		r.GET("/sitemap.xml", h.SitemapXML)  // 站点地图
		// ==========================================
		// API（为桌面客户端准备）
		// ==========================================
		api := r.Group("/api")
		api.Use(middleware.CacheControl(5 * time.Minute)) // 5 min
		{
			api.GET("/posts", h.APIPosts)
			api.GET("/posts/:slug", h.APIPost)
		}

		r.GET("/verification.html", func(c *gin.Context) {
			c.HTML(http.StatusOK, "verification.html", gin.H{})
		})
	}

	// ==========================================
	// 管理后台路由 (/admin)
	// ==========================================
	adminGroup(r, cfg, h)

	// ==========================================
	// 404 兜底
	// ==========================================
	r.NoRoute(func(c *gin.Context) {
		// 静态资源走系统 404
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

	return r
}

// ==========================================
// 管理后台路由组
// ==========================================
func adminGroup(r *gin.Engine, cfg *config.Config, h *handlers.Handler) {
	admin := r.Group("/admin")

	// 无需认证
	admin.GET("/login", h.AdminLoginPage)
	admin.POST("/login", h.AdminLogin)

	// 需要认证
	protected := admin.Group("")
	protected.Use(middleware.AdminAuth(cfg.AdminPassword))
	{
		protected.GET("/", h.AdminDashboard)                            // 仪表盘
		protected.GET("/posts/new", h.AdminNewPost)                     // 新建文章
		protected.POST("/posts", h.AdminCreatePost)                     // 保存文章
		protected.GET("/posts/:id/edit", h.AdminEditPost)               // 编辑文章
		protected.POST("/posts/:id", h.AdminUpdatePost)                 // 更新文章
		protected.POST("/posts/:id/delete", h.AdminDeletePost)          // 删除文章
		protected.GET("/posts/quick", h.AdminQuickPublish)              // 快速发布表单
		protected.POST("/posts/quick", h.AdminQuickCreatePost)          // 保存快速发布          // 删除文章
		protected.GET("/categories", h.AdminCategories)                 // 分类管理
		protected.POST("/categories", h.AdminCreateCategory)            // 新建分类
		protected.POST("/categories/:id", h.AdminUpdateCategory)        // 更新分类
		protected.POST("/categories/:id/delete", h.AdminDeleteCategory) // 删除分类
		protected.GET("/stickers", h.AdminStickers)                     // 表情包管理
		protected.POST("/stickers", h.AdminCreateSticker)               // 上传表情包
		protected.POST("/stickers/:id/delete", h.AdminDeleteSticker)    // 删除表情包
		protected.POST("/upload", h.AdminUpload)                        // 文件上传
		protected.POST("/upload/cleanup", h.AdminCleanupUploads)        // 清理未关联的上传文件
		protected.GET("/logout", h.AdminLogout)                         // 退出登录
	}
}

// ==========================================
// 模板函数注册
// ==========================================
func templateFuncMap(cfg *config.Config, baseDir string, store storage.Storage) template.FuncMap {
	return template.FuncMap{
		// 静态资源版本号（缓存破坏）
		"asset": func(path string) string {
			return path + "?v=" + cfg.AssetVersion
		},
		// 日期格式化
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006")
		},
		"formatDateShort": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		// HTML 安全输出
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		// 文本截断
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		// int64 → int 转换（模板 eq 比较用）
		"toInt": func(i int64) int { return int(i) },
		"isVideoURL": func(url string) bool {
			lower := strings.ToLower(url)
			return strings.Contains(lower, ".mp4") || strings.Contains(lower, ".webm") ||
				strings.Contains(lower, ".ogg") || strings.Contains(lower, ".mov") ||
				strings.Contains(lower, "/video/")
		},
		// 根据 category ID 查名称
		// 给文章正文 <img> 注入 loading="lazy"（兼容已有旧文章）
		// 同时将存储 URL 替换为 data-mid 索引（URL 存入当前页面的共享 MediaMap）
		"lazyImages": func(html string) template.HTML {
			html = handlers.InjectLazyLoading(html)
			html = handlers.InjectImageDimensions(html, store)
			html = handlers.InjectVideoDimensions(html, store)
			return template.HTML(handlers.BuildMediaMapWith(html, store, cfg, handlers.CurrentMediaMap()))
		},
		// 缩略图：输出 data-mid 索引 + 宽高，URL 存在 JS 数组里
		"thumbnailImg": func(url, alt string) template.HTML {
			return template.HTML(handlers.ThumbnailMidImage(url, alt, store, cfg))
		},
		// 将存储 URL 转为 data-mid 索引（URL 存入当前页面的 MediaMap）
		"mediaID": func(url string) string {
			return handlers.AddThumbMid(url, handlers.CurrentMediaMap(), store, cfg)
		},
		// 视频缩略图：Blob 模式用 data-mid，直连模式用 src
		"videoThumb": func(url string) template.HTML {
			return template.HTML(handlers.VideoThumb(url, store, cfg))
		},
		// 媒体保护 JS 配置脚本 + 初始化当前页面的 MediaMap
		"mediaConfigScript": func() template.HTML {
			mm := handlers.NewMediaMap()
			handlers.SetCurrentMediaMap(mm)
			return template.HTML(handlers.BuildMediaConfigScript(cfg))
		},
		// 当前页面 MediaMap 的加密 URL 脚本（放在 </body> 前）
		"mediaURLScript": func() template.HTML {
			mm := handlers.CurrentMediaMap()
			if mm == nil {
				return ""
			}
			return mm.Script(handlers.PageAESKey())
		},
		// 阅读时长（根据分类 slug）
		"readTime": func(categorySlug string) string {
			switch categorySlug {
			case "quick-peek":
				return "30秒"
			case "bathroom-break":
				return "3-5分钟"
			case "lunch-break":
				return "10-15分钟"
			case "daily-highlight":
				return "5-10分钟"
			default:
				return "约5分钟"
			}
		},
		// 分类 Emoji
		"catEmoji": func(categorySlug string) string {
			switch categorySlug {
			case "quick-peek":
				return "⚡"
			case "bathroom-break":
				return "☕"
			case "lunch-break":
				return "🍱"
			case "daily-highlight":
				return "🔥"
			default:
				return ""
			}
		},
		// 是否未来时间（管理后台判断定时发布）
		"isFuture": func(nt sql.NullTime) bool {
			return nt.Valid && nt.Time.After(time.Now())
		},
		"catName": func(catID sql.NullInt64, categories []models.Category) string {
			if !catID.Valid {
				return "—"
			}
			for _, c := range categories {
				if int64(c.ID) == catID.Int64 {
					return c.Name
				}
			}
			return "—"
		},
	}
}
