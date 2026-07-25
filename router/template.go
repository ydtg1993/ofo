package router

import (
	"html/template"
	"strings"
	"time"

	"ofo/config"
	"ofo/handlers"
	"ofo/handlers/admin"
	"ofo/storage"
)

// templateFuncMap returns the template.FuncMap used by all templates.
func templateFuncMap(cfg *config.Config, store storage.Storage) template.FuncMap {
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
			if !cfg.MediaProtection {
				html = handlers.RewriteContentURLs(html, cfg)
			}
			html = admin.InjectLazyLoading(html)
			html = admin.InjectImageDimensions(html, store)
			html = admin.InjectVideoDimensions(html, store)
			if cfg.MediaProtection {
				return template.HTML(handlers.BuildMediaMapWith(html, store, cfg, handlers.CurrentMediaMap()))
			}
			return template.HTML(html)
		},
		// 缩略图：输出 data-mid 索引 + 宽高，URL 存在 JS 数组里
		"thumbnailImg": func(url, alt string) template.HTML {
			return template.HTML(admin.ThumbnailMidImage(url, alt, store, cfg))
		},
		// 将存储 URL 转为 data-mid 索引（URL 存入当前页面的 MediaMap）
		"mediaID": func(url string) string {
			return handlers.AddThumbMid(url, handlers.CurrentMediaMap(), store, cfg)
		},
		// 视频缩略图：Blob 模式用 data-mid，直连模式用 src
		"videoThumb": func(url string) template.HTML {
			return template.HTML(admin.VideoThumb(url, store, cfg))
		},
		"displayURL": func(path string) string { return handlers.DisplayURL(path, cfg) },
		// 页面级媒体配置脚本（session cookie + AES 密钥）
		// 仅 MediaProtection 启用时输出；否则返回空字符串
		"mediaConfig": func() template.HTML {
			mm := handlers.NewMediaMap()
			handlers.SetCurrentMediaMap(mm)
			return template.HTML(handlers.BuildMediaConfigScript(cfg))
		},
		// 当前页面的 MediaMap 已加密脚本（含所有 data-mid → URL 映射）
		"mediaMap": func() template.HTML {
			mm := handlers.CurrentMediaMap()
			if mm == nil {
				return ""
			}
			return mm.Script(handlers.PageAESKey())
		},
		// 标签级联：判断 post_tags 中是否包含某 tagID
		"tagSelected": func(tagVal string, tags []interface{}) bool {
			// 从 template 传进来的是 models.Tag[]
			for _, t := range tags {
				switch v := t.(type) {
				case struct {
					ID   int
					Name string
					Slug string
				}:
					if tagVal == v.Name {
						return true
					}
				}
			}
			return false
		},
	}
}
