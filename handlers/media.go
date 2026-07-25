package handlers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"ofo/config"
	"ofo/logger"
	"ofo/storage"

	"github.com/gin-gonic/gin"
)

// maxMediaSize is the maximum file size the media proxy will buffer into memory
// to support Range requests (for video seeking). Files larger than this are
// streamed without Range support.
const maxMediaSize = 50 * 1024 * 1024 // 50 MB

// extToContentType maps lowercase file extensions to HTTP Content-Type values.
var extToContentType = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
}

// ---- Media Proxy Handler ----

// MediaProxy serves media files through a token-authenticated proxy.
// Route pattern: GET /media/*filepath?t=<token>
//
// The token is an HMAC-SHA256 signature that binds a timestamp to the file path,
// valid for a configurable TTL. This prevents direct file URL sharing and
// forces clients to go through the proxy with a fresh page-issued token.
func (h *Handler) MediaProxy(c *gin.Context) {
	// Extract file path — Gin's *filepath param includes leading slash
	fp := strings.TrimPrefix(c.Param("filepath"), "/")
	if fp == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// Clean the path to prevent directory traversal (use path.Clean for URL paths)
	fp = path.Clean(fp)
	// Only allow uploads/ paths
	if !strings.HasPrefix(fp, "uploads/") {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Validate session cookie — URL token alone is not enough
	sessCookie, _ := c.Cookie("ofo_m")
	if sessCookie == "" || !validatePageToken(sessCookie, h.mediaSecret(), h.Cfg.MediaTokenTTL) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Validate URL token
	token := c.Query("t")
	if token == "" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	secret := h.mediaSecret()
	if !validateMediaToken(fp, token, secret, h.Cfg.MediaTokenTTL) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Determine Content-Type from extension
	ext := strings.ToLower(filepath.Ext(fp))
	contentType, ok := extToContentType[ext]
	if !ok {
		contentType = "application/octet-stream"
	}

	// Read file from storage
	ctx := c.Request.Context()
	rc, err := h.Storage.Get(ctx, fp)
	if err != nil {
		logger.WarnWithContext(c, "media proxy: file not found", "path", fp, "err", err)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer rc.Close()

	// Read into buffer to support Range requests (up to maxMediaSize)
	// We limit the read to prevent memory exhaustion on large files.
	body, err := io.ReadAll(io.LimitReader(rc, maxMediaSize+1))
	if err != nil {
		logger.ErrorWithContext(c, "media proxy: failed to read file", "path", fp, "err", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if len(body) > maxMediaSize {
		// File too large — stream without Range support
		c.DataFromReader(http.StatusOK, int64(len(body)), contentType,
			io.MultiReader(bytes.NewReader(body), rc), nil)
		return
	}

	// Serve with Range support (needed for video seeking)
	c.Header("Content-Type", contentType)
	c.Header("Accept-Ranges", "bytes")
	c.Header("X-Content-Type-Options", "nosniff")

	// Cache for the remaining token validity (or at least 60 seconds)
	http.ServeContent(c.Writer, c.Request, filepath.Base(fp), time.Now(),
		bytes.NewReader(body))
}

// mediaSecret returns the configured secret, or derives one from AdminPassword.
func (h *Handler) mediaSecret() string {
	if h.Cfg.MediaSecret != "" {
		return h.Cfg.MediaSecret
	}
	// Fallback: derive from admin password (stable within a deployment)
	return "ofo-media-" + h.Cfg.AdminPassword
}

// ---- Token Generation & Validation ----

// GenerateMediaToken creates a signed token string for the given file path.
// Format: "<timestamp_hex>:<signature_hex>"
//   - timestamp_hex = lower-case hex encoding of Unix time
//   - signature_hex = HMAC-SHA256(timestamp_hex + ":" + filepath, secret)
func GenerateMediaToken(filepath, secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 16)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + ":" + filepath))
	sig := hex.EncodeToString(mac.Sum(nil))
	return ts + ":" + sig
}

// validateMediaToken verifies the token is authentic and not expired.
func validateMediaToken(filepath, token, secret string, ttl int) bool {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return false
	}
	tsHex, sigHex := parts[0], parts[1]

	// Parse timestamp
	ts, err := strconv.ParseInt(tsHex, 16, 64)
	if err != nil {
		return false
	}

	// Check expiry
	elapsed := time.Now().Unix() - ts
	if elapsed < 0 || elapsed > int64(ttl) {
		return false
	}

	// Verify signature with constant-time comparison
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsHex + ":" + filepath))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// ---- URL Transformation ----

// reStorageSrc matches src or data-src attributes pointing to storage-managed files.
// Group 1: the URL value. Matches both /uploads/... (new) and /static/uploads/... (legacy).
var reStorageSrc = regexp.MustCompile(
	`(?:src|data-src)\s*=\s*"(/(?:(?:static/)?(?:uploads)/[^"'<>\s]+))"`,
)

// reUploadPath matches relative upload/sticker paths in HTML for URL rewriting.
var reUploadPath = regexp.MustCompile(`/(?:uploads)/[^"'<>\s]+`)

// MediaMap collects proxy URLs during HTML processing and assigns each
// a random short ID. The ID is used as data-mid in HTML; the ID→URL
// mapping is injected as a JS object. This keeps URLs out of HTML and
// makes sequential enumeration impossible.
type MediaMap struct {
	entries map[string]string // randomID → proxyURL
}

// NewMediaMap creates a MediaMap for use during a single page render.
func NewMediaMap() *MediaMap {
	return &MediaMap{entries: make(map[string]string)}
}

// Add stores a proxy URL and returns a random 8-char hex ID for data-mid.
func (mm *MediaMap) Add(url string) string {
	id := randomMID()
	mm.entries[id] = url
	return id
}

// Script returns a <script> tag with AES-256-GCM encrypted URL mapping.
// The key is injected separately via BuildMediaConfigScript. Decryption
// happens in media-blob.js using the Web Crypto API.
func (mm *MediaMap) Script(key []byte) template.HTML {
	if len(mm.entries) == 0 || key == nil {
		return ""
	}
	plain, _ := json.Marshal(mm.entries)
	enc, err := aesEncrypt(plain, key)
	if err != nil {
		return ""
	}
	// enc = base64(nonce || ciphertext) — nonce is 12 bytes for GCM
	return template.HTML(fmt.Sprintf(
		`<script>window.__OFO_MEDIA__=window.__OFO_MEDIA__||{};window.__OFO_MEDIA__.d=%q;</script>`,
		enc,
	))
}

// aesEncrypt encrypts plaintext with AES-256-GCM and returns base64(nonce || ciphertext).
func aesEncrypt(plain, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	// Seal appends ciphertext to nonce: nonce || ciphertext || tag
	out := gcm.Seal(nonce, nonce, plain, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// randomMID generates a short random hex ID for data-mid attributes.
func randomMID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ProxyMediaURL converts a storage URL to a signed proxy URL.
//   - /static/uploads/file.jpg → /media/uploads/file.jpg?t=<token>
//   - /static/uploads/file.gif → /media/uploads/file.gif?t=<token>
//   - Non-storage URLs are returned unchanged.
func ProxyMediaURL(originalURL string, store storage.Storage, cfg *config.Config) string {
	if !cfg.MediaProtection {
		return originalURL
	}
	if !store.IsStorageURL(originalURL) {
		return originalURL
	}

	// Extract the key from the URL
	// /static/uploads/path.jpg → uploads/path.jpg
	// https://cdn.example.com/uploads/path.jpg → uploads/path.jpg
	key := extractStorageKey(originalURL, store)
	if key == "" {
		return originalURL
	}

	secret := cfg.MediaSecret
	if secret == "" {
		secret = "ofo-media-" + cfg.AdminPassword
	}

	token := GenerateMediaToken(key, secret)
	return "/media/" + key + "?t=" + token
}

// ---- Global per-page MediaMap ----
// Since Go template functions can't share per-request state, we use an atomic
// pointer that gets set before each page render and cleared after. Gin renders
// templates synchronously in a single goroutine per request, so this is safe.

var currentMM atomic.Pointer[MediaMap]

// SetCurrentMediaMap stores the MediaMap for the current page render.
func SetCurrentMediaMap(mm *MediaMap) { currentMM.Store(mm) }

// CurrentMediaMap returns the MediaMap for the current page render.
func CurrentMediaMap() *MediaMap { return currentMM.Load() }

// BuildMediaMapWith replaces all storage URLs in HTML with data-mid="N" index
// attributes using the provided MediaMap. Returns the modified HTML.
func BuildMediaMapWith(html string, store storage.Storage, cfg *config.Config, mm *MediaMap) string {
	if !cfg.MediaProtection || mm == nil {
		return html
	}

	secret := cfg.MediaSecret
	if secret == "" {
		secret = "ofo-media-" + cfg.AdminPassword
	}

	makeProxyURL := func(url string) string {
		key := extractStorageKey(url, store)
		if key == "" {
			return ""
		}
		token := GenerateMediaToken(key, secret)
		return "/media/" + key + "?t=" + token
	}

	// Pass 1: relative paths (/uploads/..., /static/uploads/...)
	html = reStorageSrc.ReplaceAllStringFunc(html, func(match string) string {
		sub := reStorageSrc.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		url := sub[1]
		if !store.IsStorageURL(url) {
			return match
		}
		proxyURL := makeProxyURL(url)
		if proxyURL == "" {
			return match
		}
		return fmt.Sprintf(`data-mid="%s"`, mm.Add(proxyURL))
	})

	// Pass 2: absolute CDN URLs from any domain (legacy data, handles old Qiniu
	// URLs when storage backend has been switched, etc.)
	reAbsSrc := regexp.MustCompile(
		`(?:src|data-src)\s*=\s*"(https?://[^/"'\s]+/(?:uploads)/[^"'<>\s]+)"`,
	)
	html = reAbsSrc.ReplaceAllStringFunc(html, func(match string) string {
		sub := reAbsSrc.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		url := sub[1]
		proxyURL := makeProxyURL(url)
		if proxyURL == "" {
			return match
		}
		return fmt.Sprintf(`data-mid="%s"`, mm.Add(proxyURL))
	})

	return html
}

// AddThumbMid generates a proxy URL for a thumbnail, adds it to the MediaMap,
// and returns the data-mid index string for use in a template.
func AddThumbMid(url string, mm *MediaMap, store storage.Storage, cfg *config.Config) string {
	if url == "" || mm == nil || !cfg.MediaProtection || !store.IsStorageURL(url) {
		return ""
	}
	key := extractStorageKey(url, store)
	if key == "" {
		return ""
	}
	secret := cfg.MediaSecret
	if secret == "" {
		secret = "ofo-media-" + cfg.AdminPassword
	}
	token := GenerateMediaToken(key, secret)
	proxyURL := "/media/" + key + "?t=" + token
	return mm.Add(proxyURL)
}

// extractStorageKey extracts the storage key from a URL.
//
//	/uploads/path.jpg → uploads/path.jpg
//	/static/uploads/path.jpg → uploads/path.jpg
//	https://cdn.example.com/uploads/path.jpg → uploads/path.jpg
func extractStorageKey(url string, store storage.Storage) string {
	// Remove /static/ or /media/ prefix first, then extract
	clean := url
	for _, prefix := range []string{"/static/", "/media/"} {
		if strings.HasPrefix(clean, prefix) {
			clean = clean[len(prefix):]
			break
		}
	}

	// Now extract from clean path: uploads/xxx or stickers/xxx
	for _, prefix := range []string{"uploads/", "uploads/"} {
		if strings.HasPrefix(clean, prefix) {
			return clean // e.g. "uploads/path.jpg"
		}
	}

	// For CDN URLs (e.g. https://cdn.example.com/uploads/path.jpg)
	if store.IsStorageURL(url) {
		for _, prefix := range []string{"/uploads/", "/stickers/"} {
			if idx := strings.Index(url, prefix); idx >= 0 {
				return url[idx+1:] // e.g. "uploads/path.jpg"
			}
		}
	}

	return ""
}

// isMediaURL checks if a URL is a proxied media URL (used by dimension injection).
func isMediaURL(url string) bool {
	return strings.HasPrefix(url, "/media/uploads/") ||
		strings.HasPrefix(url, "/media/uploads/")
}

// originalFromMediaURL tries to reverse a proxied media URL back to the
// underlying storage URL, so dimension injection can still work.
func originalFromMediaURL(url string) string {
	// Strip token query param
	if idx := strings.Index(url, "?"); idx >= 0 {
		url = url[:idx]
	}
	// /media/uploads/path.jpg → /uploads/path.jpg
	if strings.HasPrefix(url, "/media/") {
		return "/" + strings.TrimPrefix(url, "/media/")
	}
	return url
}

// IsStorageOrMediaURL returns true for both direct storage URLs and proxied
// media URLs. Used by dimension injection to determine whether to try reading
// the file's dimensions.
func IsStorageOrMediaURL(url string, store storage.Storage) bool {
	if store.IsStorageURL(url) {
		return true
	}
	if isMediaURL(url) {
		return true
	}
	return false
}

// GetMediaURLForDimension resolves the actual storage URL from either a direct
// URL or a proxied /media/ URL, for use with storage.GetMediaInfo.
func GetMediaURLForDimension(url string) string {
	if isMediaURL(url) {
		return originalFromMediaURL(url)
	}
	return url
}

// validatePageToken verifies a page token (from cookie) is authentic and not expired.
func validatePageToken(token, secret string, ttl int) bool {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return false
	}
	windowHex, sigHex := parts[0], parts[1]
	window, err := strconv.ParseInt(windowHex, 16, 64)
	if err != nil {
		return false
	}
	// Check the token is still within its validity window
	now := time.Now().Unix() / int64(ttl)
	if now-window > 1 || window-now > 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("page:" + windowHex))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sigHex), []byte(expected))
}

// GeneratePageToken creates a session-scoped token that the page can use to
// prove it was served by this server. This is embedded as a meta tag and used
// by the JavaScript to know media protection is active.
func GeneratePageToken(cfg *config.Config) string {
	if !cfg.MediaProtection {
		return ""
	}
	secret := cfg.MediaSecret
	if secret == "" {
		secret = "ofo-media-" + cfg.AdminPassword
	}
	// A page token is just a signature of the current time window
	window := strconv.FormatInt(time.Now().Unix()/int64(cfg.MediaTokenTTL), 16)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("page:" + window))
	return window + ":" + hex.EncodeToString(mac.Sum(nil))
}

// BuildMediaConfigScript returns a <script> tag that sets the session cookie
// and window.__OFO_MEDIA__ with config + AES decryption key.
func BuildMediaConfigScript(cfg *config.Config) string {
	if !cfg.MediaProtection {
		return ""
	}
	pageToken := GeneratePageToken(cfg)
	// Generate a random AES-256 key per page render
	aesKey := make([]byte, 32) // AES-256
	rand.Read(aesKey)
	keyB64 := base64.StdEncoding.EncodeToString(aesKey)

	// Store key for Script() to use
	setPageAESKey(aesKey)

	return fmt.Sprintf(
		`<script>document.cookie="ofo_m=%s;path=/;max-age=%d;SameSite=Strict";window.__OFO_MEDIA__={enabled:true,ttl:%d,k:%q};</script>`,
		pageToken, cfg.MediaTokenTTL, cfg.MediaTokenTTL, keyB64,
	)
}

// ---- Per-page AES key storage ----
var currentAESKey atomic.Pointer[[]byte]

func setPageAESKey(key []byte) {
	k := make([]byte, len(key))
	copy(k, key)
	currentAESKey.Store(&k)
}

func getPageAESKey() []byte {
	p := currentAESKey.Load()
	if p == nil {
		return nil
	}
	return *p
}

// PageAESKey returns the current page's AES decryption key (for template use).
func PageAESKey() []byte { return getPageAESKey() }

// ---- Content URL Normalization ----

var reCDNURL *regexp.Regexp

func initCDNURLPattern(domain string) {
	if domain != "" {
		reCDNURL = regexp.MustCompile(regexp.QuoteMeta(domain) + `/(uploads)/[^"'<>\s]+`)
	}
}

// NormalizeContentURLs replaces full CDN URLs in HTML content with relative
// storage paths (e.g. "https://cdn.example.com/uploads/x.jpg" → "/uploads/x.jpg").
// For local storage the HTML is returned unchanged.
func NormalizeContentURLs(html string, store storage.Storage, cfg *config.Config) string {
	if store.IsLocal() || cfg.QiniuDomain == "" {
		return html
	}
	if reCDNURL == nil {
		initCDNURLPattern(strings.TrimRight(cfg.QiniuDomain, "/"))
	}
	if reCDNURL == nil {
		return html
	}
	return reCDNURL.ReplaceAllStringFunc(html, func(match string) string {
		domain := strings.TrimRight(cfg.QiniuDomain, "/")
		return strings.TrimPrefix(match, domain)
	})
}

// reAnyCDNURL matches any absolute CDN/storage URL pointing to uploads/ or stickers/.
var reAnyCDNURL = regexp.MustCompile(`https?://[^/"'\s]+/(uploads)/[^"'<>\s]+`)

// RewriteContentURLs converts relative storage paths in HTML to public display
// URLs. Local → prepends /static, Qiniu → prepends CDN domain.
// Also handles legacy absolute URLs from any CDN domain.
func RewriteContentURLs(html string, cfg *config.Config) string {
	if cfg.StorageBackend == "qiniu" && cfg.QiniuDomain != "" {
		domain := strings.TrimRight(cfg.QiniuDomain, "/")
		html = reUploadPath.ReplaceAllStringFunc(html, func(match string) string {
			return domain + match
		})
	} else {
		// Local: /uploads/... → /static/uploads/... (Gin static serving)
		html = reUploadPath.ReplaceAllStringFunc(html, func(match string) string {
			return "/static" + match
		})
		// Legacy: any absolute CDN URL → /static/uploads/...
		// (handles old Qiniu CDN URLs when storage backend has been switched to local)
		html = reAnyCDNURL.ReplaceAllStringFunc(html, func(match string) string {
			if i := strings.Index(match, "/uploads/"); i >= 0 {
				return "/static" + match[i:]
			}
			if i := strings.Index(match, "/stickers/"); i >= 0 {
				return "/static" + match[i:]
			}
			return match
		})
	}
	return html
}

// DisplayURL converts a relative storage path to a full display URL.
// e.g. "/uploads/x.jpg" → local: "/static/uploads/x.jpg", qiniu: "https://cdn.example.com/uploads/x.jpg"
// Already-full URLs (http://...) pass through unchanged to prevent double-prepending.
func DisplayURL(relativePath string, cfg *config.Config) string {
	if relativePath == "" {
		return ""
	}
	// Already an absolute URL — return as-is (prevents double-prepending for legacy data)
	if strings.HasPrefix(relativePath, "http://") || strings.HasPrefix(relativePath, "https://") {
		return relativePath
	}
	if cfg.StorageBackend == "qiniu" && cfg.QiniuDomain != "" {
		return strings.TrimRight(cfg.QiniuDomain, "/") + relativePath
	}
	return "/static" + relativePath
}

// URLToRelativePath converts a display URL back to the canonical relative path.
// e.g. "https://cdn.example.com/uploads/x.jpg" → "/uploads/x.jpg"
// "/static/uploads/x.jpg" → "/uploads/x.jpg"
// Already-canonical paths pass through unchanged.
func URLToRelativePath(url string, store storage.Storage, cfg *config.Config) string {
	// Already canonical (starts with /uploads/ or /stickers/)
	if strings.HasPrefix(url, "/uploads/") || strings.HasPrefix(url, "/stickers/") {
		return url
	}
	// /static/uploads/... → /uploads/...
	if strings.HasPrefix(url, "/static/") {
		return strings.TrimPrefix(url, "/static")
	}
	// CDN domain URL
	if cfg.StorageBackend == "qiniu" && cfg.QiniuDomain != "" {
		domain := strings.TrimRight(cfg.QiniuDomain, "/")
		if strings.HasPrefix(url, domain+"/") {
			return strings.TrimPrefix(url, domain)
		}
	}
	return url
}
