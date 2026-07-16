package storage

import (
	"context"
	"io"
)

// Storage abstracts file operations so the application can switch between
// local disk and cloud object storage (e.g. Qiniu).
type Storage interface {
	// Upload stores a file and returns its public URL.
	// key is the object path, e.g. "uploads/<uuid>.ext" or "stickers/<uuid>.ext".
	Upload(ctx context.Context, key string, reader io.Reader, size int64) (url string, err error)

	// Delete removes a file by its key. Must not error if the object does not exist.
	Delete(ctx context.Context, key string) error

	// Get returns a reader for the object content.
	// Used for dimension extraction fallback when the storage backend doesn't
	// provide a dedicated metadata API.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// GetMediaInfo returns (width, height) for an image or video identified by its
	// public URL (as stored in the DB). Returns (0, 0, nil) for non-media files.
	GetMediaInfo(url string) (width int, height int, err error)

	// IsStorageURL returns true if the given URL is managed by this storage backend.
	// Used by SyncPostResources and dimension injection to identify own URLs.
	IsStorageURL(url string) bool

	// IsLocal returns true when storage is the local filesystem.
	// Controls whether Gin serves static files and whether directories are created.
	IsLocal() bool
}
