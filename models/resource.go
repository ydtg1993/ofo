package models

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"ofo/logger"
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
// Matches both local paths and CDN URLs, with or without date-based subdirectories.
// Examples: /static/uploads/uuid.jpg, /static/uploads/2026/07/uuid.jpg,
// https://cdn.example.com/uploads/2026/07/uuid.webp
var reCandidateUpload = regexp.MustCompile(`(?:/static/(?:uploads|stickers)/|https?://[^/\s"'<>]+/(?:uploads|stickers)/)[^"'<>\s]+`)

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

// FindOrphanedByURL returns an unlinked resource (post_id IS NULL) by its URL.
// Returns nil if not found or if the resource is already linked to a post.
func (m *ResourceModel) FindOrphanedByURL(url string) (*Resource, error) {
	var r Resource
	err := m.DB.QueryRow(
		`SELECT id, post_id, filename, url, file_size, mime_type, created_at
		 FROM resources WHERE url = ? AND post_id IS NULL`, url,
	).Scan(&r.ID, &r.PostID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteOrphanedByURL deletes an unlinked resource record by URL.
// Returns the filename (for storage cleanup) and whether a row was actually deleted.
func (m *ResourceModel) DeleteOrphanedByURL(url string) (filename string, deleted bool, err error) {
	// Find first to get the filename for storage cleanup
	row := m.DB.QueryRow(`SELECT filename FROM resources WHERE url = ? AND post_id IS NULL`, url)
	if err := row.Scan(&filename); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	result, err := m.DB.Exec(`DELETE FROM resources WHERE url = ? AND post_id IS NULL`, url)
	if err != nil {
		return "", false, err
	}
	n, _ := result.RowsAffected()
	return filename, n > 0, nil
}

// SyncPostResources scans contentHTML and reconciles all resource records:
//  1. Extract storage URLs from the HTML.
//  2. For every resource with post_id IS NULL: if its URL appears in the HTML,
//     link it to this post; otherwise delete the file + DB record (orphan cleanup).
//  3. For resources already linked to this post that are no longer in the HTML,
//     delete the file + DB record (removed from content).
//
// The deleteResource callback should remove the file from storage; its argument
// is the resources.filename value (may include date subdirectories).
func (m *ResourceModel) SyncPostResources(postID int, contentHTML string, isStorageURL func(string) bool, deleteResource func(filename string) error) error {
	// Build set of storage URLs found in the HTML
	matches := reCandidateUpload.FindAllStringSubmatch(contentHTML, -1)
	urlSet := make(map[string]bool, len(matches))
	for _, match := range matches {
		u := match[0]
		if isStorageURL(u) {
			urlSet[u] = true
		}
	}

	// ---- Pass 1: handle ALL unlinked resources (post_id IS NULL) ----
	unlinked, err := m.FindUnlinked()
	if err != nil {
		return err
	}
	for _, r := range unlinked {
		if urlSet[r.URL] {
			// URL is referenced → link to this post
			if _, err := m.DB.Exec(`UPDATE resources SET post_id = ? WHERE id = ?`, postID, r.ID); err != nil {
				return err
			}
			logger.Info("linked resource to post", "filename", r.Filename, "postID", postID)
		} else {
			// URL not referenced → orphan, delete file + record
			if err := deleteResource(r.Filename); err != nil {
				logger.Error("failed to delete orphan resource file", "filename", r.Filename, "err", err)
			}
			if _, err := m.DB.Exec(`DELETE FROM resources WHERE id = ?`, r.ID); err != nil {
				return err
			}
			logger.Info("deleted orphan resource", "filename", r.Filename)
		}
	}

	// ---- Pass 2: handle resources previously linked to this post ----
	current, err := m.FindByPostID(postID)
	if err != nil {
		return err
	}
	for _, r := range current {
		if !urlSet[r.URL] {
			// No longer in the HTML → delete file + record
			if err := deleteResource(r.Filename); err != nil {
				logger.Error("failed to delete removed resource file", "filename", r.Filename, "err", err)
			}
			if _, err := m.DB.Exec(`DELETE FROM resources WHERE id = ?`, r.ID); err != nil {
				return err
			}
			logger.Info("deleted removed resource", "filename", r.Filename, "postID", postID)
		}
	}

	return nil
}

// FindUnlinked returns all resources that are not linked to any post.
func (m *ResourceModel) FindUnlinked() ([]Resource, error) {
	rows, err := m.DB.Query(
		`SELECT id, post_id, filename, url, file_size, mime_type, created_at
		 FROM resources WHERE post_id IS NULL`,
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
			logger.Warn("error looking up resource during disk scan", "path", relPath, "err", err)
			return nil
		}
		if existing != nil {
			return nil // already tracked
		}

		fileSize := info.Size()
		ext := filepath.Ext(relPath)
		mimeType := MIMEType(ext)

		if _, err := m.Create(relPath, url, fileSize, mimeType); err != nil {
			logger.Warn("error inserting resource during disk scan", "path", relPath, "err", err)
			return nil
		}
		count++
		return nil
	})

	return count, err
}
