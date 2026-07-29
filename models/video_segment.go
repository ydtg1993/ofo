package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// VideoSegment stores the TS segment file paths for an HLS video resource.
// Segments is a JSON array of relative storage paths, e.g.
// ["uploads/2026/07/uuid/uuid_000.ts", "uploads/2026/07/uuid/uuid_001.ts"].
type VideoSegment struct {
	ID         int    `gorm:"primaryKey;autoIncrement"`
	ResourceID int    `gorm:"column:resource_id;uniqueIndex;not null"`
	Segments   string `gorm:"column:segments;type:text;not null"`
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
