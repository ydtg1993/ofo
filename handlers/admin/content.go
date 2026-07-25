package admin

import (
	"fmt"
	"regexp"
	"strings"

	"ofo/config"
	"ofo/handlers"
	"ofo/storage"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

// ---- Markdown / HTML Rendering ----

// sanitizePolicy returns a bluemonday policy that allows HTML and video elements.
func sanitizePolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// 允许视频元素
	p.AllowElements("video", "source")
	p.AllowAttrs("src", "controls", "width", "height", "autoplay", "loop", "muted", "poster").OnElements("video")
	p.AllowAttrs("src", "type").OnElements("source")
	// 允许常用 HTML 布局标签 + 样式/类名
	p.AllowElements("div", "span", "section", "article", "header", "footer", "nav", "aside", "main", "figure", "figcaption", "details", "summary")
	p.AllowAttrs("class", "id", "style").OnElements("div", "span", "section", "article", "header", "footer", "nav", "aside", "main", "figure", "figcaption", "details", "summary")
	// 允许图片宽高和样式
	p.AllowAttrs("width", "height", "style").OnElements("img")
	p.AllowAttrs("class", "id", "style").Globally()
	// 允许 iframe（嵌入视频等）
	p.AllowElements("iframe")
	p.AllowAttrs("src", "width", "height", "frameborder", "allowfullscreen", "allow", "style").OnElements("iframe")
	return p
}

// renderMarkdown converts markdown to sanitized HTML.
func renderMarkdown(md string) string {
	// 预处理：确保块引用、标题、列表前有空行
	md = normalizeMarkdown(md)
	// 预处理：递归渲染 HTML 容器标签内的 Markdown（如 <div>![](url)</div>）
	md = renderHTMLContainers(md)

	// 若全文都是纯 HTML（无 Markdown 语法），直接传给 sanitizer，
	// 跳过 blackfriday，避免 <img> 等被套上 <p>。
	var unsafe []byte
	if looksLikePureHTML(md) {
		unsafe = []byte(md)
	} else {
		unsafe = blackfriday.Run([]byte(md))
	}

	html := string(sanitizePolicy().SanitizeBytes(unsafe))
	// 给正文图片加懒加载
	html = InjectLazyLoading(html)
	return html
}

// ---- Image Lazy Loading ----

// reImgTag matches an <img> tag body for lazy-load injection.
var reImgTag = regexp.MustCompile(`<img\s([^>]*?)>`)

// InjectLazyLoading adds loading="lazy" to every <img> that lacks a loading attribute.
// Exported so the router's template function can reuse it for existing posts.
func InjectLazyLoading(html string) string {
	return reImgTag.ReplaceAllStringFunc(html, func(match string) string {
		if strings.Contains(match, "loading=") {
			return match
		}
		return strings.Replace(match, "<img ", "<img loading=\"lazy\" ", 1)
	})
}

// ---- Image Dimension Injection (lazy loading fix) ----

// reImgSrc extracts the src attribute value from an <img> tag.
var reImgSrc = regexp.MustCompile(`src\s*=\s*"([^"]*)"`)

// InjectImageDimensions adds width and height attributes to <img> tags
// whose src points to a storage-managed file, reading dimensions via the
// Storage interface. External URLs and data URIs are skipped.
func InjectImageDimensions(html string, store storage.Storage) string {
	return reImgTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src
		m := reImgSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process storage-managed URLs
		if !store.IsStorageURL(src) {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		w, h, err := store.GetMediaInfo(src)
		if err != nil || w == 0 || h == 0 {
			// Unreadable or not media -- leave tag untouched
			return match
		}

		return injectWidthHeight(match, w, h)
	})
}

// injectWidthHeight adds width, height, and aspect-ratio attributes into an img/video tag.
// The aspect-ratio inline style ensures the browser reserves the correct space
// even when src is removed by the lazy-loading script (e.g. slow network).
func injectWidthHeight(tag string, w, h int) string {
	// Build aspect-ratio inline style — this is the key to preventing layout shift
	ratioStyle := fmt.Sprintf("aspect-ratio:%d/%d", w, h)

	// Merge with existing style attribute if present
	if idx := strings.Index(tag, "style=\""); idx >= 0 {
		// Find closing quote of existing style value
		closeQuote := strings.Index(tag[idx+7:], "\"")
		if closeQuote >= 0 {
			tag = tag[:idx+7+closeQuote] + ";" + ratioStyle + tag[idx+7+closeQuote:]
		}
	} else {
		// Insert style attribute before closing >
		styleAttr := fmt.Sprintf(` style="%s"`, ratioStyle)
		if strings.HasSuffix(tag, "/>") {
			tag = tag[:len(tag)-2] + styleAttr + "/>"
		} else {
			tag = tag[:len(tag)-1] + styleAttr + ">"
		}
	}

	// Add width/height HTML attributes
	dims := fmt.Sprintf(` width="%d" height="%d"`, w, h)
	if strings.HasSuffix(tag, "/>") {
		return tag[:len(tag)-2] + dims + "/>"
	}
	return tag[:len(tag)-1] + dims + ">"
}

// ---- Video Dimension Injection ----

// reVideoTag matches a <video> opening tag.
var reVideoTag = regexp.MustCompile(`<video\s([^>]*?)>`)

// reVideoSrc extracts the src attribute value from a <video> tag.
var reVideoSrc = regexp.MustCompile(`src\s*=\s*"([^"]*)"`)

// InjectVideoDimensions adds width and height attributes to <video> tags
// whose src points to a storage-managed file, reading dimensions via the
// Storage interface. External URLs and data URIs are skipped.
func InjectVideoDimensions(html string, store storage.Storage) string {
	return reVideoTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src from the opening <video> tag
		m := reVideoSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process storage-managed URLs
		if !store.IsStorageURL(src) {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		w, h, err := store.GetMediaInfo(src)
		if err != nil || w == 0 || h == 0 {
			// Unreadable or not media -- leave tag untouched
			return match
		}

		return injectWidthHeight(match, w, h)
	})
}

// ---- Thumbnail Helpers ----

// ThumbnailMidImage generates an <img> tag with data-mid index + dimensions.
// The actual proxy URL is stored in the page's MediaMap (JS array).
func ThumbnailMidImage(url, alt string, store storage.Storage, cfg *config.Config) string {
	if url == "" {
		return ""
	}
	dimURL := url
	if handlers.IsStorageOrMediaURL(url, store) {
		dimURL = handlers.GetMediaURLForDimension(url)
	}

	if cfg.MediaProtection && store.IsStorageURL(url) {
		mm := handlers.CurrentMediaMap()
		mid := handlers.AddThumbMid(url, mm, store, cfg)
		w, h, _ := store.GetMediaInfo(dimURL)
		dims := ""
		if w > 0 && h > 0 {
			dims = fmt.Sprintf(` width="%d" height="%d" style="aspect-ratio:%d/%d"`, w, h, w, h)
		}
		return fmt.Sprintf(`<img data-mid="%s" alt="%s" loading="lazy"%s>`, mid, alt, dims)
	}

	html := fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy">`, handlers.DisplayURL(dimURL, cfg), alt)
	html = InjectImageDimensions(html, store)
	return html
}

// VideoThumb generates a <video> tag for a thumbnail URL.
// When media protection is on, emits data-mid for blob loading.
// When off, emits src with the direct URL (CDN-friendly).
func VideoThumb(url string, store storage.Storage, cfg *config.Config) string {
	if url == "" {
		return ""
	}
	if cfg.MediaProtection && store.IsStorageURL(url) {
		mm := handlers.CurrentMediaMap()
		mid := handlers.AddThumbMid(url, mm, store, cfg)
		return fmt.Sprintf(`<video data-mid="%s" preload="none"></video>`, mid)
	}
	return fmt.Sprintf(`<video src="%s" preload="none"></video>`, handlers.DisplayURL(url, cfg))
}

// ThumbnailImage generates an <img> tag for a thumbnail URL with skeleton-ready
// attributes (width, height, aspect-ratio) injected for storage-managed files.
// Used by the homepage card thumbnails.
// When media protection is enabled, emits data-media-src with a signed proxy URL
// instead of a direct src, so JS can load it as a blob.
func ThumbnailImage(url, alt string, store storage.Storage, cfg *config.Config) string {
	if url == "" {
		return ""
	}
	// Resolve the URL for dimension injection (use the underlying storage URL)
	dimURL := url
	if handlers.IsStorageOrMediaURL(url, store) {
		dimURL = handlers.GetMediaURLForDimension(url)
	}

	// Build tag with data-media-src (or src if protection disabled)
	if cfg.MediaProtection && store.IsStorageURL(url) {
		proxyURL := handlers.ProxyMediaURL(url, store, cfg)
		// Get dimensions from storage for skeleton placeholder
		w, h, _ := store.GetMediaInfo(dimURL)
		dims := ""
		if w > 0 && h > 0 {
			dims = fmt.Sprintf(` width="%d" height="%d" style="aspect-ratio:%d/%d"`, w, h, w, h)
		}
		return fmt.Sprintf(`<img data-media-src="%s" alt="%s" loading="lazy"%s>`,
			proxyURL, alt, dims)
	}

	html := fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy">`, dimURL, alt)
	html = InjectImageDimensions(html, store)
	return html
}

// ---- HTML Container Rendering ----

// HTML 容器标签集合
var htmlContainerTags = map[string]bool{
	"div": true, "section": true, "article": true, "figure": true,
	"figcaption": true, "details": true, "summary": true,
	"header": true, "footer": true, "nav": true, "aside": true, "main": true,
}

// reHTMLContainer 匹配 HTML 容器标签，捕获标签名、属性和内容。
var reHTMLContainer = regexp.MustCompile(
	`(?s)<(div|section|article|figure|figcaption|details|summary|header|footer|nav|aside|main)\b([^>]*)>(.+?)</(\w+)>`,
)

// renderHTMLContainers 递归渲染 HTML 容器内的 Markdown 内容。
// reStartsWithHTMLTag 检测内容是否以 HTML 标签开头。
var reStartsWithHTMLTag = regexp.MustCompile(`^\s*<[a-zA-Z]`)

// reHasMarkdownBlock 检测内容是否含 Markdown 块级语法（标题、引用、列表、代码块）。
var reHasMarkdownBlock = regexp.MustCompile(`(?m)^(#{1,6}\s|>\s|[\-\*\+]\s|\d+\.\s|\x60\x60\x60)`)

// looksLikePureHTML 判断内容是否纯 HTML（无需 Markdown 渲染）。
// 条件：以 < 标签开头 且 不含 Markdown 块级/内联语法。
func looksLikePureHTML(s string) bool {
	if !reStartsWithHTMLTag.MatchString(s) {
		return false
	}
	// 检查块级 Markdown 语法
	if reHasMarkdownBlock.MatchString(s) {
		return false
	}
	// 检查常见内联 Markdown 语法（加粗 ** __，链接 [text](url)，图片 ![alt](url)）
	if strings.Contains(s, "**") || strings.Contains(s, "__") ||
		strings.Contains(s, "![") {
		return false
	}
	// 检查 [text](url) 链接语法（排除 HTML 中的方括号属性如 [style]）
	if reMarkdownLink.MatchString(s) {
		return false
	}
	return true
}

// reMarkdownLink 匹配 Markdown 链接语法 [text](url)。
var reMarkdownLink = regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`)

func renderHTMLContainers(md string) string {
	for i := 0; i < 10; i++ {
		before := md
		md = reHTMLContainer.ReplaceAllStringFunc(md, func(match string) string {
			sub := reHTMLContainer.FindStringSubmatch(match)
			if len(sub) < 5 {
				return match
			}
			openTag := sub[1]
			attrs := sub[2]
			content := sub[3]
			closeTag := sub[4]

			// 只处理首尾标签匹配的
			if openTag != closeTag || !htmlContainerTags[openTag] {
				return match
			}

			// 把 width="100px" 等属性转为内联 style
			attrs = normalizeHTMLAttrs(attrs)

			// 如果内容看起来是纯 HTML（无 Markdown 语法），跳过渲染，
			// 避免 <img> 等标签被 blackfriday 套上 <p>。
			var rendered string
			if looksLikePureHTML(content) {
				rendered = content
			} else {
				rendered = string(blackfriday.Run([]byte(content)))
			}
			return "<" + openTag + attrs + ">\n" + rendered + "\n</" + openTag + ">"
		})
		if md == before {
			break
		}
	}
	return md
}

// reAttrWidth 匹配模板中直接写的 width="100px" / height="200" 等属性
var reAttrWidth = regexp.MustCompile(`(?i)\b(width|height)\s*=\s*"(\d+%?)"`)

var reAttrAlign = regexp.MustCompile(`(?i)\b(align)\s*=\s*"(left|center|right)"`)

// normalizeHTMLAttrs 把 width/height/align 等 HTML 废弃属性转为 inline style。
func normalizeHTMLAttrs(attrs string) string {
	attrs = reAttrWidth.ReplaceAllStringFunc(attrs, func(m string) string {
		parts := reAttrWidth.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		prop := strings.ToLower(parts[1])
		val := parts[2]
		return "style=\"" + prop + ":" + val + "\""
	})
	attrs = reAttrAlign.ReplaceAllStringFunc(attrs, func(m string) string {
		parts := reAttrAlign.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		return "style=\"text-align:" + strings.ToLower(parts[2]) + "\""
	})
	return attrs
}

// normalizeMarkdown 预处理 markdown，让非空行前的 > / # / - 能正确解析。
var reBlockNeedsBlank = regexp.MustCompile(`(?m)^([^\n>#\-\s].+)\n(> |#{1,6} |\d+\. |\- )`)

func normalizeMarkdown(md string) string {
	// 统一换行符：\r\n (Windows) / \r (old Mac) → \n
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	// 在块级元素前补空行（如果前一非空行不是空行或另一个块元素）
	return reBlockNeedsBlank.ReplaceAllString(md, "$1\n\n$2")
}
