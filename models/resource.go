package models

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Resource represents an uploaded file tracked in the database.
type Resource struct {
	ID        int
	PostID    sql.NullInt64
	Filename  string
	URL       string
	FileSize  int64
	MimeType  string
	CreatedAt time.Time
}

// ResourceModel wraps database access for resources.
type ResourceModel struct {
	DB *sql.DB
}

// extToMIME maps lowercase file extensions to MIME types.
var extToMIME = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",
	".mov":  "video/quicktime",
}

// MIMEType returns the MIME type for a given file extension.
func MIMEType(ext string) string {
	if m, ok := extToMIME[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// Regular expression to extract candidate upload URLs from HTML content.
// Matches both local paths (/static/uploads/...) and CDN URLs (https://.../uploads/...).
var reCandidateUpload = regexp.MustCompile(`(?:/static/(?:uploads|stickers)/|https?://[^/\s"'<>]+/(?:uploads|stickers)/)([a-f0-9\-]+\.[a-z0-9]+)`)

// Create inserts a new resource record with post_id = NULL.
func (m *ResourceModel) Create(filename, url string, fileSize int64, mimeType string) (int64, error) {
	result, err := m.DB.Exec(
		`INSERT INTO resources (filename, url, file_size, mime_type) VALUES (?, ?, ?, ?)`,
		filename, url, fileSize, mimeType,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FindByPostID returns all resources linked to a post.
func (m *ResourceModel) FindByPostID(postID int) ([]Resource, error) {
	rows, err := m.DB.Query(
		`SELECT id, post_id, filename, url, file_size, mime_type, created_at
		 FROM resources WHERE post_id = ?`, postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []Resource
	for rows.Next() {
		var r Resource
		if err := rows.Scan(&r.ID, &r.PostID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.CreatedAt); err != nil {
			return nil, err
		}
		resources = append(resources, r)
	}
	return resources, rows.Err()
}

// FindByURL returns a single resource by its URL.
func (m *ResourceModel) FindByURL(url string) (*Resource, error) {
	var r Resource
	err := m.DB.QueryRow(
		`SELECT id, post_id, filename, url, file_size, mime_type, created_at
		 FROM resources WHERE url = ?`, url,
	).Scan(&r.ID, &r.PostID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteByPostID removes all resource records for a post (filesystem cleanup is caller's responsibility).
func (m *ResourceModel) DeleteByPostID(postID int) error {
	_, err := m.DB.Exec(`DELETE FROM resources WHERE post_id = ?`, postID)
	return err
}

// Delete removes a single resource record by ID.
func (m *ResourceModel) Delete(id int) error {
	_, err := m.DB.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}

// SyncPostResources scans contentHTML for storage-managed URLs and links/unlinks
// resource records to the given postID. The isStorageURL callback identifies which
// URLs belong to the configured storage backend.
func (m *ResourceModel) SyncPostResources(postID int, contentHTML string, isStorageURL func(string) bool) error {
	// Extract all candidate upload URLs from the HTML
	matches := reCandidateUpload.FindAllStringSubmatch(contentHTML, -1)
	urlSet := make(map[string]bool, len(matches))
	for _, match := range matches {
		// match[0] is the full URL like /static/uploads/uuid.ext or https://cdn.example.com/uploads/uuid.ext
		u := match[0]
		if isStorageURL(u) {
			urlSet[u] = true
		}
	}

	// Get currently linked resources for this post
	current, err := m.FindByPostID(postID)
	if err != nil {
		return err
	}

	// Unlink resources that are no longer in the HTML
	for _, r := range current {
		if !urlSet[r.URL] {
			if _, err := m.DB.Exec(`UPDATE resources SET post_id = NULL WHERE id = ?`, r.ID); err != nil {
				return err
			}
		}
	}

	// Link new resources found in the HTML that are currently unlinked
	for url := range urlSet {
		if _, err := m.DB.Exec(
			`UPDATE resources SET post_id = ? WHERE url = ? AND post_id IS NULL`,
			postID, url,
		); err != nil {
			return err
		}
	}

	return nil
}

// ScanDiskAndRecord scans the uploads directory (including date-based subdirectories)
// and creates resource records for files that don't have one yet.
// Returns the number of newly recorded files.
func (m *ResourceModel) ScanDiskAndRecord(uploadsDir string) (int, error) {
	count := 0

	err := filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Relative path from uploadsDir (e.g. "2026/07/uuid.ext" or just "uuid.ext")
		relPath, err := filepath.Rel(uploadsDir, path)
		if err != nil {
			relPath = info.Name()
		}
		// Normalize to forward slashes
		relPath = filepath.ToSlash(relPath)
		url := "/static/uploads/" + relPath

		// Check if record already exists
		existing, err := m.FindByURL(url)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("ScanDiskAndRecord: error looking up %s: %v", relPath, err)
			return nil
		}
		if existing != nil {
			return nil // already tracked
		}

		fileSize := info.Size()
		ext := filepath.Ext(relPath)
		mimeType := MIMEType(ext)

		if _, err := m.Create(relPath, url, fileSize, mimeType); err != nil {
			log.Printf("ScanDiskAndRecord: error inserting %s: %v", relPath, err)
			return nil
		}
		count++
		return nil
	})

	return count, err
}
