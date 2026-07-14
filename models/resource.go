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

// Regular expression to extract upload URLs from HTML content.
var reStaticUpload = regexp.MustCompile(`/static/uploads/([a-f0-9\-]+\.[a-z0-9]+)`)

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

// SyncPostResources scans contentHTML for /static/uploads/ URLs and links/unlinks
// resource records to the given postID.
func (m *ResourceModel) SyncPostResources(postID int, contentHTML string) error {
	// Extract all upload URLs from the HTML
	matches := reStaticUpload.FindAllStringSubmatch(contentHTML, -1)
	urlSet := make(map[string]bool, len(matches))
	for _, match := range matches {
		// match[0] is the full path like /static/uploads/uuid.ext
		urlSet[match[0]] = true
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

// ScanDiskAndRecord scans the uploads directory and creates resource records
// for files that don't have one yet. Returns the number of newly recorded files.
func (m *ResourceModel) ScanDiskAndRecord(uploadsDir string) (int, error) {
	entries, err := os.ReadDir(uploadsDir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		url := "/static/uploads/" + filename

		// Check if record already exists
		existing, err := m.FindByURL(url)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("ScanDiskAndRecord: error looking up %s: %v", filename, err)
			continue
		}
		if existing != nil {
			continue // already tracked
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			log.Printf("ScanDiskAndRecord: error stating %s: %v", filename, err)
			continue
		}

		fileSize := info.Size()
		ext := filepath.Ext(filename)
		mimeType := MIMEType(ext)

		if _, err := m.Create(filename, url, fileSize, mimeType); err != nil {
			log.Printf("ScanDiskAndRecord: error inserting %s: %v", filename, err)
			continue
		}
		count++
	}

	return count, nil
}
