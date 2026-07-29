package models

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"ofo/logger"

	"gorm.io/gorm"
)

// Resource represents an uploaded file tracked in the database.
type Resource struct {
	ID          int    `gorm:"primaryKey;autoIncrement"`
	Filename    string `gorm:"size:255;not null"`
	URL         string `gorm:"size:512;not null"`
	FileSize    int64  `gorm:"column:file_size;default:0"`
	MimeType    string `gorm:"column:mime_type;size:100"`
	Storage     string `gorm:"size:16;default:local"`
	CreatedAt   time.Time
	Posts       []Post     `gorm:"many2many:post_resources"`
	LinkedPosts []PostCard `gorm:"-"` // populated on list queries only
}

// ResourceModel wraps GORM database access for resources.
type ResourceModel struct {
	DB *gorm.DB
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
var reCandidateUpload = regexp.MustCompile(`(?:(?:/static)?/uploads/|https?://[^/\s"'<>]+/uploads/)[^"'<>\s]+`)

// Create inserts a new resource record.
func (m *ResourceModel) Create(filename, url, storage string, fileSize int64, mimeType string) (int64, error) {
	r := Resource{
		Filename: filename,
		URL:      url,
		FileSize: fileSize,
		MimeType: mimeType,
		Storage:  storage,
	}
	if err := m.DB.Create(&r).Error; err != nil {
		return 0, err
	}
	return int64(r.ID), nil
}

// FindByURL returns a single resource by its URL.
func (m *ResourceModel) FindByURL(url string) (*Resource, error) {
	var r Resource
	if err := m.DB.Where("url = ?", url).First(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// Delete removes a single resource record and its join-table entries by ID.
func (m *ResourceModel) Delete(id int) error {
	// Clear associations
	m.DB.Where("resource_id = ?", id).Delete(&struct {
		PostID     int `gorm:"column:post_id"`
		ResourceID int `gorm:"column:resource_id"`
	}{})
	return m.DB.Delete(&Resource{ID: id}).Error
}

// ---- Join-table (post_resources) methods ----

// FindResourcesByPostID returns all resources linked to a post via the join table.
func (m *ResourceModel) FindResourcesByPostID(postID int) ([]Resource, error) {
	var resources []Resource
	if err := m.DB.Model(&Post{ID: postID}).Association("Resources").Find(&resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// LinkPostResource creates an association between a post and a resource.
func (m *ResourceModel) LinkPostResource(postID, resourceID int) error {
	return m.DB.Model(&Post{ID: postID}).Association("Resources").Append(&Resource{ID: resourceID})
}

// UnlinkPostResources removes all resource associations for a post.
func (m *ResourceModel) UnlinkPostResources(postID int) error {
	return m.DB.Model(&Post{ID: postID}).Association("Resources").Clear()
}

// CountLinkedPosts returns the number of posts linked to a resource.
func (m *ResourceModel) CountLinkedPosts(resourceID int) (int, error) {
	var count int64
	if err := m.DB.Table("post_resources").Where("resource_id = ?", resourceID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
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
// is the URL value (which includes the full storage path).
func (m *ResourceModel) SyncPostResources(postID int, contentHTML string, isStorageURL func(string) bool, deleteResource func(url string) error) error {
	// Build set of storage URLs found in the HTML
	matches := reCandidateUpload.FindAllStringSubmatch(contentHTML, -1)
	urlSet := make(map[string]bool, len(matches))
	for _, match := range matches {
		u := match[0]
		if isStorageURL(u) {
			urlSet[u] = true
		}
	}

	// Pass 1: Link resources whose URLs appear in the HTML
	for url := range urlSet {
		r, err := m.FindByURL(url)
		if err != nil {
			continue // resource not tracked yet
		}
		if err := m.LinkPostResource(postID, r.ID); err != nil {
			return err
		}
	}

	// Pass 2: Unlink resources that are no longer in the HTML
	current, err := m.FindResourcesByPostID(postID)
	if err != nil {
		return err
	}
	for _, r := range current {
		if !urlSet[r.URL] {
			// No longer referenced — unlink
			m.DB.Where("post_id = ? AND resource_id = ?", postID, r.ID).Delete(&struct {
				PostID     int `gorm:"column:post_id"`
				ResourceID int `gorm:"column:resource_id"`
			}{})
			// If this resource is now completely orphaned, delete file + record
			noLinks, _ := m.HasNoLinks(r.ID)
			if noLinks {
				if err := deleteResource(r.URL); err != nil {
					logger.Error("failed to delete orphan resource file", "url", r.URL, "err", err)
				}
				m.DB.Delete(&Resource{ID: r.ID})
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
	var list []Resource
	if err := m.DB.Order("created_at DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}

	for i := range list {
		list[i].LinkedPosts, _ = m.linkedPostsForResource(list[i].ID)
	}
	return list, nil
}

// linkedPostsForResource returns post cards linked to a resource.
func (m *ResourceModel) linkedPostsForResource(resourceID int) ([]PostCard, error) {
	var posts []Post
	if err := m.DB.
		Joins("JOIN post_resources pr ON posts.id = pr.post_id").
		Where("pr.resource_id = ?", resourceID).
		Find(&posts).Error; err != nil {
		return nil, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = PostCard{
			ID:    p.ID,
			Title: p.Title,
			Slug:  p.Slug,
		}
	}
	return cards, nil
}

// CountAll returns the total number of resources.
func (m *ResourceModel) CountAll() (int, error) {
	var total int64
	err := m.DB.Model(&Resource{}).Count(&total).Error
	return int(total), err
}

// ---- Disk scan ----

// ScanDiskAndRecord scans the uploads directory and creates resource records
// for files that don't have one yet. Returns the number of newly recorded files.
func (m *ResourceModel) ScanDiskAndRecord(uploadsDir string) (int, error) {
	count := 0

	err := filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		filename := info.Name()
		relPath, _ := filepath.Rel(uploadsDir, path)
		url := "/uploads/" + filepath.ToSlash(relPath)

		// Check if record already exists
		existing, err := m.FindByURL(url)
		if err != nil && err != gorm.ErrRecordNotFound {
			logger.Warn("error looking up resource during disk scan", "path", filename, "err", err)
			return nil
		}
		if existing != nil {
			return nil // already tracked
		}

		fileSize := info.Size()
		ext := filepath.Ext(filename)
		mimeType := MIMEType(ext)

		if _, err := m.Create(filename, url, "local", fileSize, mimeType); err != nil {
			logger.Warn("error inserting resource during disk scan", "path", filename, "err", err)
			return nil
		}
		count++
		return nil
	})

	return count, err
}
