package handlers

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for image.DecodeConfig
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	_ "image/png"  // register PNG decoder for image.DecodeConfig
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"ofo/middleware"
	"ofo/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/image/webp"
)

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
	unsafe := blackfriday.Run([]byte(md))
	html := string(sanitizePolicy().SanitizeBytes(unsafe))
	// 给正文图片加懒加载
	html = InjectLazyLoading(html)
	return html
}

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

// imageDimCache caches image dimensions keyed by URL path.
var imageDimCache sync.Map

// reImgSrc extracts the src attribute value from an <img> tag.
var reImgSrc = regexp.MustCompile(`src\s*=\s*"([^"]*)"`)

// getImageDimensions reads the width and height of an image file from disk.
func getImageDimensions(imgPath string) (int, int, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(imgPath))
	switch ext {
	case ".webp":
		cfg, err := webp.DecodeConfig(f)
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	default:
		// JPEG, PNG, GIF handled by standard library registered decoders
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	}
}

// InjectImageDimensions adds width and height attributes to <img> tags
// whose src points to a local /static/uploads/ file, reading actual
// dimensions from disk. External URLs and data URIs are skipped.
// Results are cached to avoid repeated file I/O.
func InjectImageDimensions(html, baseDir string) string {
	return reImgTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src
		m := reImgSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process local uploads
		if !strings.HasPrefix(src, "/static/uploads/") {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		// Consult cache
		type dimPair struct{ W, H int }
		if v, ok := imageDimCache.Load(src); ok {
			d := v.(dimPair)
			return injectWidthHeight(match, d.W, d.H)
		}

		// Read from disk
		filename := filepath.Base(src)
		imgPath := filepath.Join(baseDir, "static", "uploads", filename)

		w, h, err := getImageDimensions(imgPath)
		if err != nil {
			// File missing or unreadable -- leave tag untouched
			return match
		}

		imageDimCache.Store(src, dimPair{w, h})
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

// getVideoDimensions reads the width and height of a video file from disk.
// Supports MP4/MOV (ISO BMFF) and WebM (Matroska) containers.
func getVideoDimensions(videoPath string) (int, int, error) {
	ext := strings.ToLower(filepath.Ext(videoPath))
	switch ext {
	case ".mp4", ".mov", ".m4v":
		return getMP4Dimensions(videoPath)
	case ".webm", ".mkv":
		return getWebMDimensions(videoPath)
	default:
		return 0, 0, fmt.Errorf("unsupported video format: %s", ext)
	}
}

// getMP4Dimensions parses an MP4/MOV file and returns the first video track's dimensions.
// MP4 uses ISO Base Media File Format (ISO/IEC 14496-12).
// Video dimensions are stored in the tkhd (track header) atom inside moov > trak.
func getMP4Dimensions(path string) (int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	// Find moov atom
	moov := findAtom(data, "moov")
	if moov == nil {
		return 0, 0, fmt.Errorf("moov atom not found")
	}

	// Scan trak atoms inside moov
	offset := 0
	for offset+8 <= len(moov) {
		size := binary.BigEndian.Uint32(moov[offset : offset+4])
		atype := string(moov[offset+4 : offset+8])
		if size < 8 || int(size) > len(moov)-offset {
			break
		}
		if atype == "trak" {
			w, h, ok := parseTrak(moov[offset+8 : offset+int(size)])
			if ok {
				return w, h, nil
			}
		}
		offset += int(size)
	}

	return 0, 0, fmt.Errorf("no video track found in moov")
}

// findAtom locates a top-level atom by type in ISO BMFF data.
func findAtom(data []byte, target string) []byte {
	offset := 0
	for offset+8 <= len(data) {
		size := binary.BigEndian.Uint32(data[offset : offset+4])
		if size < 8 || int(size) > len(data)-offset {
			break
		}
		atype := string(data[offset+4 : offset+8])
		if atype == target {
			return data[offset+8 : offset+int(size)]
		}
		offset += int(size)
	}
	return nil
}

// parseTrak extracts video dimensions from a trak atom.
// Looks for tkhd (dimensions) and mdia/hdlr (handler type = 'vide').
func parseTrak(trak []byte) (w, h int, ok bool) {
	offset := 0
	hFound := false

	for offset+8 <= len(trak) {
		size := binary.BigEndian.Uint32(trak[offset : offset+4])
		atype := string(trak[offset+4 : offset+8])
		if size < 8 || int(size) > len(trak)-offset {
			break
		}
		body := trak[offset+8 : offset+int(size)]

		switch atype {
		case "tkhd":
			w, h = parseTkhd(body)
		case "mdia":
			if isVideoTrack(body) {
				hFound = true
			}
		}

		if hFound && w > 0 && h > 0 {
			return w, h, true
		}
		offset += int(size)
	}
	return 0, 0, false
}

// parseTkhd extracts width and height from a tkhd (track header) atom.
// Width/height are 32-bit fixed-point 16.16 values.
func parseTkhd(tkhd []byte) (w, h int) {
	if len(tkhd) < 84 {
		return 0, 0
	}
	version := tkhd[0]
	var widthOffset, heightOffset int
	if version == 1 {
		widthOffset = 84
		heightOffset = 88
	} else {
		widthOffset = 76
		heightOffset = 80
	}
	if len(tkhd) < heightOffset+4 {
		return 0, 0
	}
	// Fixed-point 16.16: integer part is in upper 16 bits
	wRaw := binary.BigEndian.Uint32(tkhd[widthOffset : widthOffset+4])
	hRaw := binary.BigEndian.Uint32(tkhd[heightOffset : heightOffset+4])
	return int(wRaw >> 16), int(hRaw >> 16)
}

// isVideoTrack checks if a mdia atom contains a video handler ('vide').
func isVideoTrack(mdia []byte) bool {
	hdlr := findAtom(mdia, "hdlr")
	if hdlr == nil || len(hdlr) < 12 {
		return false
	}
	// hdlr atom: version(1) + flags(3) + pre_defined(4) = 8 bytes header
	// handler_type is at offset 8-11
	return len(hdlr) >= 12 && string(hdlr[8:12]) == "vide"
}

// getWebMDimensions parses a WebM/Matroska file and returns video dimensions.
// WebM uses EBML format; video width/height are in the Video sub-element of TrackEntry.
func getWebMDimensions(path string) (int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	// Skip EBML header (first element) — find Segment
	seg := findEBMLElement(data, 0x18538067) // Segment ID
	if seg == nil {
		return 0, 0, fmt.Errorf("Segment not found in WebM")
	}

	// Find Tracks inside Segment
	tracks := findEBMLElement(seg, 0x1654AE6B) // Tracks ID
	if tracks == nil {
		return 0, 0, fmt.Errorf("Tracks not found in WebM")
	}

	// Find first TrackEntry
	trackEntry := findEBMLElement(tracks, 0xAE) // TrackEntry ID
	if trackEntry == nil {
		return 0, 0, fmt.Errorf("TrackEntry not found in WebM")
	}

	// Check TrackType is video (1)
	trackType := findEBMLElement(trackEntry, 0x83) // TrackType ID
	if trackType == nil || len(trackType) < 1 || trackType[0] != 1 {
		return 0, 0, fmt.Errorf("no video track in WebM")
	}

	// Find Video sub-element inside TrackEntry
	videoElem := findEBMLElement(trackEntry, 0xE0) // Video ID
	if videoElem == nil {
		return 0, 0, fmt.Errorf("Video element not found in WebM")
	}

	// Read PixelWidth (0xB0) and PixelHeight (0xBA)
	pw := findEBMLElement(videoElem, 0xB0)
	ph := findEBMLElement(videoElem, 0xBA)
	if pw == nil || ph == nil {
		return 0, 0, fmt.Errorf("video dimensions not found in WebM")
	}

	w := readEBMLUint(pw)
	h := readEBMLUint(ph)
	if w == 0 || h == 0 {
		return 0, 0, fmt.Errorf("invalid video dimensions in WebM")
	}
	return w, h, nil
}

// findEBMLElement locates an EBML element by ID and returns its data.
// Simple implementation: scans for element ID, reads variable-length size, returns body.
func findEBMLElement(data []byte, elemID uint32) []byte {
	idBytes := encodeEBMLID(elemID)
	offset := 0
	for offset+len(idBytes) <= len(data) {
		// Check for element ID match at current position
		match := true
		for i := 0; i < len(idBytes); i++ {
			if data[offset+i] != idBytes[i] {
				match = false
				break
			}
		}
		if match {
			pos := offset + len(idBytes)
			bodySize, sizeLen := readVInt(data[pos:])
			if sizeLen == 0 || pos+sizeLen+int(bodySize) > len(data) {
				offset++
				continue
			}
			start := pos + sizeLen
			end := start + int(bodySize)
			return data[start:end]
		}
		offset++
	}
	return nil
}

// encodeEBMLID encodes a uint32 EBML element ID to bytes (big-endian, no leading zeros).
func encodeEBMLID(id uint32) []byte {
	if id < 0x80 {
		return []byte{byte(id)}
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], id)
	// Find first non-zero byte
	for i := 0; i < 4; i++ {
		if b[i] != 0 {
			return b[i:]
		}
	}
	return []byte{0}
}

// readVInt reads a variable-length integer (EBML VINT) and returns (value, bytesRead).
func readVInt(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}
	first := data[0]
	// Count leading zeros to determine length
	length := 1
	mask := byte(0x80)
	for mask > 0 && (first&mask) == 0 {
		length++
		mask >>= 1
	}
	if length > len(data) {
		return 0, 0
	}
	val := uint64(first & (mask - 1))
	for i := 1; i < length; i++ {
		val = (val << 8) | uint64(data[i])
	}
	return val, length
}

// readEBMLUint interprets EBML element body as a big-endian unsigned integer.
func readEBMLUint(data []byte) int {
	val := 0
	for _, b := range data {
		val = (val << 8) | int(b)
	}
	return val
}

// InjectVideoDimensions adds width and height attributes to <video> tags
// whose src points to a local /static/uploads/ file, reading actual
// dimensions from disk. External URLs and data URIs are skipped.
func InjectVideoDimensions(html, baseDir string) string {
	return reVideoTag.ReplaceAllStringFunc(html, func(match string) string {
		// Extract src from the opening <video> tag (may also be on <source> children,
		// but the common case is src directly on <video>)
		m := reVideoSrc.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		src := m[1]

		// Only process local uploads
		if !strings.HasPrefix(src, "/static/uploads/") {
			return match
		}

		// Skip if this tag already has both width and height
		hasW := strings.Contains(match, "width=")
		hasH := strings.Contains(match, "height=")
		if hasW && hasH {
			return match
		}

		// Consult cache — reuse the same cache as images (keyed by URL)
		type dimPair struct{ W, H int }
		if v, ok := imageDimCache.Load(src); ok {
			d := v.(dimPair)
			return injectWidthHeight(match, d.W, d.H)
		}

		// Read from disk
		filename := filepath.Base(src)
		videoPath := filepath.Join(baseDir, "static", "uploads", filename)

		w, h, err := getVideoDimensions(videoPath)
		if err != nil {
			// File missing or unreadable — leave tag untouched
			return match
		}

		imageDimCache.Store(src, dimPair{w, h})
		return injectWidthHeight(match, w, h)
	})
}

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

			rendered := blackfriday.Run([]byte(content))
			return "<" + openTag + attrs + ">\n" + string(rendered) + "\n</" + openTag + ">"
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

// AdminPageData holds data for admin template rendering.
type AdminPageData struct {
	Title          string
	Cfg            interface{}
	Error          string
	Success        string
	Posts          []models.Post
	Post           *models.Post
	Categories     []models.Category
	Tags           []models.Tag
	AllTags        []models.Tag
	IsEditing      bool
	IsNew          bool
	ShowCategories bool
	ShowStickers   bool
	Stickers       []models.Sticker
	Pagination     *models.Pagination
}

// ---- Login ----

func (h *Handler) AdminLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_login.html", gin.H{
		"Cfg":   h.Cfg,
		"Error": "",
	})
}

func (h *Handler) AdminLogin(c *gin.Context) {
	password := c.PostForm("password")
	if password != h.Cfg.AdminPassword {
		c.HTML(http.StatusUnauthorized, "admin_login.html", gin.H{
			"Cfg":   h.Cfg,
			"Error": "密码错误",
		})
		return
	}
	middleware.SetAdminCookie(c, h.Cfg.AdminPassword)
	c.Redirect(http.StatusFound, "/admin")
}

// ---- Logout ----

func (h *Handler) AdminLogout(c *gin.Context) {
	middleware.ClearAdminCookie(c)
	c.Redirect(http.StatusFound, "/admin/login")
}

// ---- Dashboard ----

func (h *Handler) AdminDashboard(c *gin.Context) {
	total, _ := h.PostModel.CountAll()
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage

	posts, err := h.PostModel.ListAllPaginated(offset, pg.PerPage)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "admin_dashboard.html", AdminPageData{
			Title: "Dashboard",
			Cfg:   h.Cfg,
			Error: "加载文章列表失败",
		})
		return
	}

	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Pagination: pg,
	})
}

// ---- New Post Form ----

func (h *Handler) AdminNewPost(c *gin.Context) {
	categories, _ := h.PostModel.AllCategoriesSimple()
	allTags, _ := h.PostModel.AllTagsSimple()

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:      "New Post",
		Cfg:        h.Cfg,
		IsNew:      true,
		Categories: categories,
		AllTags:    allTags,
	})
}

// ---- Edit Post Form ----

func (h *Handler) AdminEditPost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	post, err := h.PostModel.GetByID(id)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	categories, _ := h.PostModel.AllCategoriesSimple()
	tags, _ := h.PostModel.TagsForPost(id)
	allTags, _ := h.PostModel.AllTagsSimple()

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:      "Edit: " + post.Title,
		Cfg:        h.Cfg,
		Post:       post,
		Categories: categories,
		Tags:       tags,
		AllTags:    allTags,
		IsEditing:  true,
	})
}

// ---- Create Post ----

func (h *Handler) AdminCreatePost(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagStr := c.PostForm("tags")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	// Render markdown
	contentHTML := renderMarkdown(contentMD)

	// Excerpt
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = extractExcerptStr(contentMD, 200)
	}

	// Auto-extract thumbnail if not manually set
	if thumbnailURL == "" {
		thumbnailURL = models.ExtractThumbnail(contentHTML)
	}

	var categoryID sql.NullInt64
	if categoryIDStr != "" {
		if cid, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			categoryID = sql.NullInt64{Int64: cid, Valid: true}
		}
	}

	tagNames := parseTags(tagStr)

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))

	postID, err := h.PostModel.Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, createdAt, tagNames)
	if err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "New Post",
			Cfg:        h.Cfg,
			IsNew:      true,
			Categories: categories,
			Error:      "保存失败：" + err.Error(),
		})
		return
	}

	// 关联上传资源到文章
	if err := h.ResourceModel.SyncPostResources(int(postID), contentHTML); err != nil {
		log.Printf("AdminCreatePost: failed to sync resources for post %d: %v", postID, err)
	}

	// Redirect to dashboard with success
	h.adminDashboardWithSuccess(c, "文章发布成功")
}

// ---- Update Post ----

func (h *Handler) AdminUpdatePost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagStr := c.PostForm("tags")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	contentHTML := renderMarkdown(contentMD)
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = extractExcerptStr(contentMD, 200)
	}

	// Auto-extract thumbnail if not manually set
	if thumbnailURL == "" {
		thumbnailURL = models.ExtractThumbnail(contentHTML)
	}

	var categoryID sql.NullInt64
	if categoryIDStr != "" {
		if cid, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			categoryID = sql.NullInt64{Int64: cid, Valid: true}
		}
	}

	tagNames := parseTags(tagStr)

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))

	if err := h.PostModel.Update(id, title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, createdAt, tagNames); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		tags, _ := h.PostModel.TagsForPost(id)
		post, _ := h.PostModel.GetByID(id)
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "Edit: " + title,
			Cfg:        h.Cfg,
			Post:       post,
			Categories: categories,
			Tags:       tags,
			IsEditing:  true,
			Error:      "更新失败：" + err.Error(),
		})
		return
	}

	// 同步上传资源关联
	if err := h.ResourceModel.SyncPostResources(id, contentHTML); err != nil {
		log.Printf("AdminUpdatePost: failed to sync resources for post %d: %v", id, err)
	}

	h.adminDashboardWithSuccess(c, "文章更新成功")
}

// ---- Delete Post ----

func (h *Handler) AdminDeletePost(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// 1. 查找文章关联的资源
	resources, err := h.ResourceModel.FindByPostID(id)
	if err != nil {
		log.Printf("AdminDeletePost: failed to find resources for post %d: %v", id, err)
	}

	// 2. 删除磁盘上的资源文件
	uploadsDir := filepath.Join(h.BaseDir, "static", "uploads")
	for _, r := range resources {
		filePath := filepath.Join(uploadsDir, r.Filename)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("AdminDeletePost: failed to delete file %s: %v", filePath, err)
		}
	}

	// 3. 删除资源记录
	if err := h.ResourceModel.DeleteByPostID(id); err != nil {
		log.Printf("AdminDeletePost: failed to delete resource records for post %d: %v", id, err)
	}

	// 4. 删除文章本身
	if err := h.PostModel.Delete(id); err != nil {
		h.adminDashboardWithSuccess(c, "删除文章失败")
		return
	}

	h.adminDashboardWithSuccess(c, "文章删除成功")
}

// ---- Category Management ----

func (h *Handler) AdminCategories(c *gin.Context) {
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
		Title:          "Category Management",
		Cfg:            h.Cfg,
		Categories:     categories,
		ShowCategories: true,
	})
}

func (h *Handler) AdminCreateCategory(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}
	if name == "" {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	if err := h.PostModel.CreateCategory(name, slug); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "创建分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (h *Handler) AdminUpdateCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}

	if err := h.PostModel.UpdateCategory(id, name, slug); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "更新分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (h *Handler) AdminDeleteCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := h.PostModel.DeleteCategory(id); err != nil {
		categories, _ := h.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            h.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "删除分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

// ---- Sticker Management ----

// AdminStickers displays the sticker management page.
func (h *Handler) AdminStickers(c *gin.Context) {
	total, _ := h.StickerModel.Count()
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage

	stickers, _ := h.StickerModel.ListPaginated(offset, pg.PerPage)

	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
		Stickers:     stickers,
		Pagination:   pg,
	})
}

// AdminCreateSticker handles sticker upload (single file per request, supports AJAX).
func (h *Handler) AdminCreateSticker(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.stickerError(c, "请选择文件")
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mov": true,
	}
	if !allowed[ext] {
		h.stickerError(c, "不支持的文件类型："+ext)
		return
	}

	// Ensure stickers directory
	stickersDir := filepath.Join(h.BaseDir, "static", "stickers")
	if err := os.MkdirAll(stickersDir, 0755); err != nil {
		h.stickerError(c, "创建目录失败")
		return
	}

	// Generate unique filename
	savedName := uuid.New().String() + ext
	dstPath := filepath.Join(stickersDir, savedName)
	dst, err := os.Create(dstPath)
	if err != nil {
		h.stickerError(c, "创建文件失败")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		h.stickerError(c, "写入文件失败")
		return
	}

	url := "/static/stickers/" + savedName
	mimeType := models.MIMEType(ext)
	if _, err := h.StickerModel.Create(savedName, url, header.Size, mimeType); err != nil {
		log.Printf("AdminCreateSticker: failed to record sticker %s: %v", savedName, err)
	}

	// AJAX request → JSON response; regular form → redirect
	if c.GetHeader("X-Requested-With") == "XMLHttpRequest" || c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{"url": url, "filename": savedName})
		return
	}
	c.Redirect(http.StatusFound, "/admin/stickers")
}

// stickerError returns an error for the sticker management page (HTML or JSON).
func (h *Handler) stickerError(c *gin.Context, msg string) {
	if c.GetHeader("X-Requested-With") == "XMLHttpRequest" || c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	total, _ := h.StickerModel.Count()
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage
	stickers, _ := h.StickerModel.ListPaginated(offset, pg.PerPage)

	c.HTML(http.StatusOK, "admin_stickers.html", AdminPageData{
		Title:        "表情包管理",
		Cfg:          h.Cfg,
		ShowStickers: true,
		Stickers:     stickers,
		Pagination:   pg,
		Error:        msg,
	})
}

// AdminDeleteSticker removes a sticker by ID and deletes the file from disk.
func (h *Handler) AdminDeleteSticker(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/stickers")
		return
	}

	// 先查出记录，拿到文件名
	sticker, err := h.StickerModel.GetByID(id)
	if err != nil {
		log.Printf("AdminDeleteSticker: sticker %d not found: %v", id, err)
		c.Redirect(http.StatusFound, "/admin/stickers")
		return
	}

	// 删除磁盘文件
	filePath := filepath.Join(h.BaseDir, "static", "stickers", sticker.Filename)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("AdminDeleteSticker: failed to remove file %s: %v", filePath, err)
	}

	// 删除数据库记录
	if err := h.StickerModel.Delete(id); err != nil {
		log.Printf("AdminDeleteSticker: failed to delete sticker %d: %v", id, err)
	}

	c.Redirect(http.StatusFound, "/admin/stickers")
}

// ---- File Upload ----

func (h *Handler) AdminUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mov": true,
	}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件类型：" + ext})
		return
	}

	// Generate unique filename
	savedName := uuid.New().String() + ext
	// Determine uploads dir relative to working directory
	uploadsDir := "static/uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建上传目录失败"})
		return
	}

	dstPath := filepath.Join(uploadsDir, savedName)
	dst, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文件失败"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入文件失败"})
		return
	}

	url := "/static/uploads/" + savedName

	// 记录到资源表（post_id 暂时为空，保存文章时关联）
	mimeType := models.MIMEType(ext)
	if _, err := h.ResourceModel.Create(savedName, url, header.Size, mimeType); err != nil {
		log.Printf("AdminUpload: failed to record resource %s: %v", savedName, err)
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ---- Helpers ----

func (h *Handler) adminDashboardWithSuccess(c *gin.Context, msg string) {
	total, _ := h.PostModel.CountAll()
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage
	posts, _ := h.PostModel.ListAllPaginated(offset, pg.PerPage)
	categories, _ := h.PostModel.AllCategoriesSimple()

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        h.Cfg,
		Posts:      posts,
		Categories: categories,
		Success:    msg,
		Pagination: pg,
	})
}

// adminPagination builds Pagination info from query params.
func adminPagination(c *gin.Context, total int, perPage int) *models.Pagination {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return &models.Pagination{
		CurrentPage: page,
		TotalPages:  totalPages,
		PerPage:     perPage,
		TotalPosts:  total,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}
}

func slugifyStr(s string) string {
	result := ""
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result += strings.ToLower(string(r))
		} else if r == ' ' || r == '-' || r == '_' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
			}
		}
	}
	slug := strings.Trim(result, "-")
	// 兜底：纯中文等标题 slugify 后为空时，用时间戳
	if slug == "" {
		slug = fmt.Sprintf("post-%d", time.Now().Unix())
	}
	return slug
}

func extractExcerptStr(md string, maxLen int) string {
	// Strip markdown roughly
	replacer := strings.NewReplacer("`", "", "#", "", "*", "", "_", "", "[", "", "]", "", "(", "", ")", "")
	clean := replacer.Replace(md)
	// Remove ``` blocks
	for {
		start := strings.Index(clean, "```")
		if start < 0 {
			break
		}
		end := strings.Index(clean[start+3:], "```")
		if end < 0 {
			break
		}
		clean = clean[:start] + clean[start+3+end+3:]
	}
	clean = strings.Join(strings.Fields(clean), " ")

	if len(clean) > maxLen {
		cut := clean[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return clean
}

func parseTags(tagStr string) []string {
	if strings.TrimSpace(tagStr) == "" {
		return nil
	}
	// 按换行拆分，兼容 \n 和 \r\n
	parts := strings.FieldsFunc(tagStr, func(r rune) bool { return r == '\n' || r == '\r' })
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// parseDate parses a form date string, defaults to today.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t
		}
	}
	return time.Now()
}
