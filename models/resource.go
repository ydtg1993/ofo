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
	ID          int
	Filename    string
	URL         string
	FileSize    int64
	MimeType    string
	Storage     string // "local" or "qiniu"
	CreatedAt   time.Time
	LinkedPosts []PostCard // 关联的文章（仅列表查询时填充）
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
// Matches relative paths (/uploads/..., /static/uploads/...) and CDN URLs.
var reCandidateUpload = regexp.MustCompile(`(?:(?:/static)?/uploads/|https?://[^/\s"'<>]+/uploads/)[^"'<>\s]+`)

// Create inserts a new resource record.
func (m *ResourceModel) Create(filename, url, storage string, fileSize int64, mimeType string) (int64, error) {
	result, err := m.DB.Exec(
		`INSERT INTO resources (filename, url, file_size, mime_type, storage) VALUES (?, ?, ?, ?, ?)`,
		filename, url, fileSize, mimeType, storage,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// FindByURL returns a single resource by its URL.
func (m *ResourceModel) FindByURL(url string) (*Resource, error) {
	var r Resource
	err := m.DB.QueryRow(
		`SELECT id, filename, url, file_size, mime_type, storage, created_at
		 FROM resources WHERE url = ?`, url,
	).Scan(&r.ID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.Storage, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Delete removes a single resource record and its join-table entries by ID.
func (m *ResourceModel) Delete(id int) error {
	m.DB.Exec(`DELETE FROM post_resources WHERE resource_id = ?`, id)
	_, err := m.DB.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}

// ---- Join-table (post_resources) methods ----

// FindResourcesByPostID returns all resources linked to a post via the join table.
func (m *ResourceModel) FindResourcesByPostID(postID int) ([]Resource, error) {
	rows, err := m.DB.Query(
		`SELECT r.id, r.filename, r.url, r.file_size, r.mime_type, r.storage, r.created_at
		 FROM resources r
		 JOIN post_resources pr ON r.id = pr.resource_id
		 WHERE pr.post_id = ?`, postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []Resource
	for rows.Next() {
		var r Resource
		if err := rows.Scan(&r.ID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.Storage, &r.CreatedAt); err != nil {
			return nil, err
		}
		resources = append(resources, r)
	}
	return resources, rows.Err()
}

// LinkPostResource creates an association between a post and a resource.
func (m *ResourceModel) LinkPostResource(postID, resourceID int) error {
	_, err := m.DB.Exec(
		`INSERT IGNORE INTO post_resources (post_id, resource_id) VALUES (?, ?)`,
		postID, resourceID,
	)
	return err
}

// UnlinkPostResources removes all resource associations for a post.
func (m *ResourceModel) UnlinkPostResources(postID int) error {
	_, err := m.DB.Exec(`DELETE FROM post_resources WHERE post_id = ?`, postID)
	return err
}

// CountLinkedPosts returns the number of posts linked to a resource.
func (m *ResourceModel) CountLinkedPosts(resourceID int) (int, error) {
	var count int
	err := m.DB.QueryRow(
		`SELECT COUNT(*) FROM post_resources WHERE resource_id = ?`, resourceID,
	).Scan(&count)
	return count, err
}

// HasNoLinks returns true if the resource is not linked to any post.
func (m *ResourceModel) HasNoLinks(resourceID int) (bool, error) {
	count, err := m.CountLinkedPosts(resourceID)
	return count == 0, err
}

// ---- SyncPostResources ----

// SyncPostResources scans contentHTML and reconciles resource-post associations:
//  1. Extract storage URLs from the HTML.
//  2. For each URL found, find the matching resource and link it to the post.
//  3. Remove old associations for this post that no longer appear in the HTML.
//  4. Delete completely orphaned resources (no links + removed from content).
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

	// ---- Pass 1: Link resources whose URLs appear in the HTML ----
	for url := range urlSet {
		r, err := m.FindByURL(url)
		if err != nil {
			continue // resource not tracked yet
		}
		if err := m.LinkPostResource(postID, r.ID); err != nil {
			return err
		}
	}

	// ---- Pass 2: Unlink resources that are no longer in the HTML ----
	current, err := m.FindResourcesByPostID(postID)
	if err != nil {
		return err
	}
	for _, r := range current {
		if !urlSet[r.URL] {
			// No longer referenced → unlink
			if _, err := m.DB.Exec(`DELETE FROM post_resources WHERE post_id = ? AND resource_id = ?`, postID, r.ID); err != nil {
				return err
			}
			// If this resource is now completely orphaned, delete file + record
			noLinks, _ := m.HasNoLinks(r.ID)
			if noLinks {
				if err := deleteResource(r.Filename); err != nil {
					logger.Error("failed to delete orphan resource file", "filename", r.Filename, "err", err)
				}
				if _, err := m.DB.Exec(`DELETE FROM resources WHERE id = ?`, r.ID); err != nil {
					return err
				}
				logger.Info("deleted orphan resource", "filename", r.Filename)
			}
		}
	}

	return nil
}

// ---- List / Count ----

// ListAll returns all resources ordered by newest first with offset/limit.
// Fills LinkedPosts for each resource.
func (m *ResourceModel) ListAll(offset, limit int) ([]Resource, error) {
	rows, err := m.DB.Query(
		`SELECT id, filename, url, file_size, mime_type, storage, created_at
		 FROM resources ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Resource
	for rows.Next() {
		var r Resource
		if err := rows.Scan(&r.ID, &r.Filename, &r.URL, &r.FileSize, &r.MimeType, &r.Storage, &r.CreatedAt); err != nil {
			return nil, err
		}
		// Load linked posts
		r.LinkedPosts, _ = m.linkedPostsForResource(r.ID)
		list = append(list, r)
	}
	return list, nil
}

// linkedPostsForResource returns post cards linked to a resource via post_resources.
func (m *ResourceModel) linkedPostsForResource(resourceID int) ([]PostCard, error) {
	rows, err := m.DB.Query(
		`SELECT p.id, p.title, p.slug
		 FROM posts p
		 JOIN post_resources pr ON p.id = pr.post_id
		 WHERE pr.resource_id = ?`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []PostCard
	for rows.Next() {
		var c PostCard
		if err := rows.Scan(&c.ID, &c.Title, &c.Slug); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// CountAll returns the total number of resources.
func (m *ResourceModel) CountAll() (int, error) {
	var total int
	err := m.DB.QueryRow("SELECT COUNT(*) FROM resources").Scan(&total)
	return total, err
}

// ---- Disk scan ----

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
		url := "/uploads/" + relPath

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

		if _, err := m.Create(relPath, url, "local", fileSize, mimeType); err != nil {
			logger.Warn("error inserting resource during disk scan", "path", relPath, "err", err)
			return nil
		}
		count++
		return nil
	})

	return count, err
}
