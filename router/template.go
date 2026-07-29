package router

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"ofo/config"
	"ofo/handlers"
	"ofo/handlers/admin"
	"ofo/models"
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
		// 将 *int 解引用为 int（用于模板比较）
		"intPtr": func(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		},
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
		// 标签级联：判断当前 tag 是否属于文章的 tags 列表（admin 编辑器用）
		"tagSelected": func(tag models.Tag, postTags []models.Tag) bool {
			for _, pt := range postTags {
				if pt.ID == tag.ID {
					return true
				}
			}
			return false
		},
		"inc": func(i int) int { return i + 1 },
		"catName": func(catID *int, categories []models.Category) string {
			if catID == nil {
				return "—"
			}
			for _, c := range categories {
				if c.ID == *catID {
					return c.Name
				}
			}
			return "—"
		},
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
		"formatSize": func(size int64) string {
			switch {
			case size >= 1<<20:
				return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
			case size >= 1<<10:
				return fmt.Sprintf("%.1f KB", float64(size)/(1<<10))
			default:
				return fmt.Sprintf("%d B", size)
			}
		},
		"hasPrefix": strings.HasPrefix,
		"isFuture":  func(nt *time.Time) bool { return nt != nil && nt.After(time.Now()) },
		"joinTags": func(tags []models.Tag) string {
			names := make([]string, len(tags))
			for i, t := range tags {
				names[i] = t.Name
			}
			return strings.Join(names, "\n")
		},
	}
}
