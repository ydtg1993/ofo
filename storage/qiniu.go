package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/go-sdk/v7/storagev2/credentials"
	"github.com/qiniu/go-sdk/v7/storagev2/objects"
	"github.com/qiniu/go-sdk/v7/storagev2/uploader"
	"github.com/qiniu/go-sdk/v7/storagev2/uptoken"
)

// QiniuStorage stores files in a Qiniu Kodo bucket with public CDN access.
type QiniuStorage struct {
	bucket     string
	domain     string // CDN domain with scheme, e.g. "https://cdn.example.com"
	uploader   uploader.Uploader
	objectsMgr *objects.ObjectsManager
	bucketObj  *objects.Bucket
	httpClient *http.Client
	mediaCache sync.Map
}

// imageInfoResp is the JSON response from ?imageInfo.
type imageInfoResp struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

// avinfoResp is the JSON response from ?avinfo.
type avinfoResp struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// NewQiniuStorage creates a Qiniu Kodo storage backend.
// domain should include the scheme, e.g. "https://cdn.example.com".
func NewQiniuStorage(accessKey, secretKey, bucket, domain string) (*QiniuStorage, error) {
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("qiniu: accessKey and secretKey are required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("qiniu: bucket is required")
	}
	if domain == "" {
		return nil, fmt.Errorf("qiniu: domain is required")
	}

	// Normalize domain: strip trailing slash
	domain = strings.TrimRight(domain, "/")

	creds := credentials.NewCredentials(accessKey, secretKey)

	// Create upload token signer with a broad-scope put policy (1 hour expiry).
	// Using a key-prefix policy so the token works for any object under the prefix.
	putPolicy, err := uptoken.NewPutPolicy(bucket, time.Now().Add(1*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("qiniu: failed to create put policy: %w", err)
	}
	signer := uptoken.NewSigner(putPolicy, creds)

	// Create form uploader
	formUploader := uploader.NewFormUploader(&uploader.FormUploaderOptions{
		UpToken: signer,
	})

	// Create objects manager and bucket reference for delete/stat operations
	objMgr := objects.NewObjectsManager(nil)

	return &QiniuStorage{
		bucket:     bucket,
		domain:     domain,
		uploader:   formUploader,
		objectsMgr: objMgr,
		bucketObj:  objMgr.Bucket(bucket),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Upload stores a file in the Qiniu bucket and returns its public CDN URL.
func (s *QiniuStorage) Upload(ctx context.Context, key string, reader io.Reader, _ int64) (string, error) {
	objectName := key // key is like "uploads/uuid.ext"
	fileName := filepath.Base(key)

	objectOptions := &uploader.ObjectOptions{
		BucketName: s.bucket,
		ObjectName: &objectName,
		FileName:   fileName,
	}

	if err := s.uploader.UploadReader(ctx, reader, objectOptions, nil); err != nil {
		return "", fmt.Errorf("qiniu upload: %w", err)
	}

	url := s.domain + "/" + key
	return url, nil
}

// Delete removes a file from the Qiniu bucket. Does not error if the object doesn't exist.
func (s *QiniuStorage) Delete(ctx context.Context, key string) error {
	op := s.bucketObj.Object(key).Delete()
	if err := s.objectsMgr.Batch(ctx, []objects.Operation{op}, nil); err != nil {
		return fmt.Errorf("qiniu delete %s: %w", key, err)
	}
	return nil
}

// Get downloads file content from the CDN. Used for dimension extraction.
func (s *QiniuStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	url := s.domain + "/" + key
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qiniu get %s: %w", key, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("qiniu get %s: HTTP %d", key, resp.StatusCode)
	}

	return resp.Body, nil
}

// IsStorageURL returns true if the URL is served by this Qiniu bucket's CDN.
func (s *QiniuStorage) IsStorageURL(url string) bool {
	return strings.Contains(url, s.domain)
}

// IsLocal returns false — Qiniu is remote storage.
func (s *QiniuStorage) IsLocal() bool { return false }

// GetMediaInfo returns image or video dimensions for a Qiniu CDN URL.
// Uses the Qiniu imageInfo / avinfo HTTP APIs.
func (s *QiniuStorage) GetMediaInfo(url string) (int, int, error) {
	// Check cache
	type dim struct{ W, H int }
	if v, ok := s.mediaCache.Load(url); ok {
		d := v.(dim)
		return d.W, d.H, nil
	}

	ext := strings.ToLower(filepath.Ext(url))
	var w, h int
	var err error

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		w, h, err = s.fetchImageInfo(url)
	case ".mp4", ".mov", ".m4v", ".webm", ".mkv", ".ogg":
		w, h, err = s.fetchAVInfo(url)
	default:
		return 0, 0, nil // not a media file
	}

	if err != nil {
		return 0, 0, err
	}

	s.mediaCache.Store(url, dim{w, h})
	return w, h, nil
}

// fetchImageInfo requests ?imageInfo and parses width/height.
func (s *QiniuStorage) fetchImageInfo(url string) (int, int, error) {
	resp, err := s.httpClient.Get(url + "?imageInfo")
	if err != nil {
		return 0, 0, fmt.Errorf("imageInfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("imageInfo: HTTP %d", resp.StatusCode)
	}

	var info imageInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, 0, fmt.Errorf("imageInfo parse: %w", err)
	}

	return info.Width, info.Height, nil
}

// fetchAVInfo requests ?avinfo and parses the first video stream's dimensions.
func (s *QiniuStorage) fetchAVInfo(url string) (int, int, error) {
	resp, err := s.httpClient.Get(url + "?avinfo")
	if err != nil {
		return 0, 0, fmt.Errorf("avinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("avinfo: HTTP %d", resp.StatusCode)
	}

	var info avinfoResp
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return 0, 0, fmt.Errorf("avinfo parse: %w", err)
	}

	for _, stream := range info.Streams {
		if stream.CodecType == "video" {
			return stream.Width, stream.Height, nil
		}
	}

	return 0, 0, fmt.Errorf("avinfo: no video stream found")
}
