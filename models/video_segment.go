package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// CoverInfo is the JSON structure stored in the cover column of video_segments.
// It records the poster image path and pixel dimensions for a video resource.
type CoverInfo struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// VideoSegment stores the TS segment file paths and cover poster info
// for a video resource (HLS or short video).
// Segments is a JSON array of relative storage paths, e.g.
// ["uploads/2026/07/uuid/uuid_000.ts", ...].  Cover is JSON of CoverInfo.
type VideoSegment struct {
	ID         int    `gorm:"primaryKey;autoIncrement"`
	ResourceID int    `gorm:"column:resource_id;uniqueIndex;not null"`
	Segments   string `gorm:"column:segments;type:text;not null"`
	Cover      string `gorm:"column:cover;type:text"` // JSON: CoverInfo
	CreatedAt  time.Time
}

// TableName overrides the default table name.
func (VideoSegment) TableName() string { return "video_segments" }

// VideoSegmentModel wraps GORM access for video_segments.
type VideoSegmentModel struct {
	DB *gorm.DB
}

// Create inserts a new video segment record.
func (m *VideoSegmentModel) Create(resourceID int, segments []string) (int64, error) {
	data, err := json.Marshal(segments)
	if err != nil {
		return 0, err
	}
	vs := VideoSegment{
		ResourceID: resourceID,
		Segments:   string(data),
	}
	if err := m.DB.Create(&vs).Error; err != nil {
		return 0, err
	}
	return int64(vs.ID), nil
}

// FindByResourceID returns the segment record for a resource, or nil if not found.
func (m *VideoSegmentModel) FindByResourceID(resourceID int) (*VideoSegment, error) {
	var vs VideoSegment
	if err := m.DB.Where("resource_id = ?", resourceID).First(&vs).Error; err != nil {
		return nil, err
	}
	return &vs, nil
}

// SegmentsList decodes the JSON array into a []string.
func (vs *VideoSegment) SegmentsList() ([]string, error) {
	var list []string
	if err := json.Unmarshal([]byte(vs.Segments), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteByResourceID removes the segment record for a resource.
func (m *VideoSegmentModel) DeleteByResourceID(resourceID int) error {
	return m.DB.Where("resource_id = ?", resourceID).Delete(&VideoSegment{}).Error
}

// CoverInfo decodes the cover JSON column into a CoverInfo struct.
func (vs *VideoSegment) CoverInfo() (*CoverInfo, error) {
	if vs.Cover == "" {
		return nil, nil
	}
	var ci CoverInfo
	if err := json.Unmarshal([]byte(vs.Cover), &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

// UpsertCover inserts or updates the cover info for a video resource.
// If no video_segments row exists yet (short video), it creates one with an
// empty segments array.
func (m *VideoSegmentModel) UpsertCover(resourceID int, ci *CoverInfo) error {
	data, err := json.Marshal(ci)
	if err != nil {
		return err
	}
	coverJSON := string(data)

	// Try update first; if no row exists, insert one with empty segments.
	result := m.DB.Model(&VideoSegment{}).
		Where("resource_id = ?", resourceID).
		Update("cover", coverJSON)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return m.DB.Create(&VideoSegment{
			ResourceID: resourceID,
			Segments:   "[]",
			Cover:      coverJSON,
		}).Error
	}
	return nil
}

// FindCoverByResourceURL looks up the cover info for a resource by its storage
// URL.  Returns nil if no cover record exists.
func (m *VideoSegmentModel) FindCoverByResourceURL(url string) (*CoverInfo, error) {
	var vs VideoSegment
	err := m.DB.
		Joins("JOIN resources ON resources.id = video_segments.resource_id").
		Where("resources.url = ?", url).
		First(&vs).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if vs.Cover == "" {
		return nil, nil
	}
	var ci CoverInfo
	if err := json.Unmarshal([]byte(vs.Cover), &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

// UpsertSegments creates or updates the segments + cover for an HLS resource.
// This is the original Create method upgraded to also store the cover.
func (m *VideoSegmentModel) UpsertSegments(resourceID int, segments []string, ci *CoverInfo) (int64, error) {
	segData, err := json.Marshal(segments)
	if err != nil {
		return 0, err
	}
	coverJSON := ""
	if ci != nil {
		coverData, err := json.Marshal(ci)
		if err != nil {
			return 0, err
		}
		coverJSON = string(coverData)
	}
	vs := VideoSegment{
		ResourceID: resourceID,
		Segments:   string(segData),
		Cover:      coverJSON,
	}
	if err := m.DB.Create(&vs).Error; err != nil {
		return 0, err
	}
	return int64(vs.ID), nil
}
