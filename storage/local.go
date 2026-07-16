package storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/image/webp"
)

// LocalStorage stores files on the local filesystem under baseDir.
type LocalStorage struct {
	baseDir    string
	mediaCache sync.Map
}

// NewLocalStorage creates a LocalStorage that writes files under baseDir.
func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

// Upload saves the file to disk and returns a relative URL path.
func (s *LocalStorage) Upload(_ context.Context, key string, reader io.Reader, _ int64) (string, error) {
	dstPath := filepath.Join(s.baseDir, key)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return "", err
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, reader); err != nil {
		return "", err
	}

	// Build URL: key is like "uploads/uuid.ext" → "/static/uploads/uuid.ext"
	url := "/static/" + key
	return url, nil
}

// Delete removes a file from disk. Does not error if the file doesn't exist.
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path := filepath.Join(s.baseDir, key)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Get opens a file for reading. The caller must close the returned reader.
func (s *LocalStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.baseDir, key)
	return os.Open(path)
}

// IsStorageURL returns true if the URL is a local static file path.
func (s *LocalStorage) IsStorageURL(url string) bool {
	return strings.HasPrefix(url, "/static/uploads/") ||
		strings.HasPrefix(url, "/static/stickers/")
}

// IsLocal returns true.
func (s *LocalStorage) IsLocal() bool { return true }

// GetMediaInfo reads image or video dimensions from a local file path.
func (s *LocalStorage) GetMediaInfo(url string) (int, int, error) {
	// Check cache
	type dim struct{ W, H int }
	if v, ok := s.mediaCache.Load(url); ok {
		d := v.(dim)
		return d.W, d.H, nil
	}

	// url is like "/static/uploads/uuid.jpg" or "/static/stickers/uuid.gif"
	if !strings.HasPrefix(url, "/static/") {
		return 0, 0, nil
	}

	// Extract relative path: "/static/uploads/uuid.jpg" → "uploads/uuid.jpg"
	relPath := strings.TrimPrefix(url, "/static/")
	filePath := filepath.Join(s.baseDir, relPath)

	ext := strings.ToLower(filepath.Ext(url))
	var w, h int
	var err error

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		w, h, err = getImageDimensions(filePath)
	case ".mp4", ".mov", ".m4v":
		w, h, err = getMP4Dimensions(filePath)
	case ".webm", ".mkv":
		w, h, err = getWebMDimensions(filePath)
	default:
		return 0, 0, nil
	}

	if err != nil {
		return 0, 0, err
	}

	s.mediaCache.Store(url, dim{w, h})
	return w, h, nil
}

// ---- Image Dimension Helpers ----

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
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	}
}

// ---- MP4/MOV Dimension Helpers ----

// getMP4Dimensions parses an MP4/MOV file and returns the first video track's dimensions.
func getMP4Dimensions(path string) (int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	moov := findAtom(data, "moov")
	if moov == nil {
		return 0, 0, fmt.Errorf("moov atom not found")
	}

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
	wRaw := binary.BigEndian.Uint32(tkhd[widthOffset : widthOffset+4])
	hRaw := binary.BigEndian.Uint32(tkhd[heightOffset : heightOffset+4])
	return int(wRaw >> 16), int(hRaw >> 16)
}

func isVideoTrack(mdia []byte) bool {
	hdlr := findAtom(mdia, "hdlr")
	if hdlr == nil || len(hdlr) < 12 {
		return false
	}
	return len(hdlr) >= 12 && string(hdlr[8:12]) == "vide"
}

// ---- WebM/MKV Dimension Helpers ----

func getWebMDimensions(path string) (int, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	seg := findEBMLElement(data, 0x18538067)
	if seg == nil {
		return 0, 0, fmt.Errorf("Segment not found in WebM")
	}

	tracks := findEBMLElement(seg, 0x1654AE6B)
	if tracks == nil {
		return 0, 0, fmt.Errorf("Tracks not found in WebM")
	}

	trackEntry := findEBMLElement(tracks, 0xAE)
	if trackEntry == nil {
		return 0, 0, fmt.Errorf("TrackEntry not found in WebM")
	}

	trackType := findEBMLElement(trackEntry, 0x83)
	if trackType == nil || len(trackType) < 1 || trackType[0] != 1 {
		return 0, 0, fmt.Errorf("no video track in WebM")
	}

	videoElem := findEBMLElement(trackEntry, 0xE0)
	if videoElem == nil {
		return 0, 0, fmt.Errorf("Video element not found in WebM")
	}

	pw := findEBMLElement(videoElem, 0xB0)
	ph := findEBMLElement(videoElem, 0xBA)
	if pw == nil || ph == nil {
		return 0, 0, fmt.Errorf("video dimensions not found in WebM")
	}

	return readEBMLUint(pw), readEBMLUint(ph), nil
}

func findEBMLElement(data []byte, elemID uint32) []byte {
	idBytes := encodeEBMLID(elemID)
	offset := 0
	for offset+len(idBytes) <= len(data) {
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

func encodeEBMLID(id uint32) []byte {
	if id < 0x80 {
		return []byte{byte(id)}
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], id)
	for i := 0; i < 4; i++ {
		if b[i] != 0 {
			return b[i:]
		}
	}
	return []byte{0}
}

func readVInt(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}
	first := data[0]
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

func readEBMLUint(data []byte) int {
	val := 0
	for _, b := range data {
		val = (val << 8) | int(b)
	}
	return val
}
