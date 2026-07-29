package models

import (
	"time"

	"gorm.io/gorm"
)

// Series represents a named collection of posts with ordering.
type Series struct {
	ID        int    `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"size:100;not null"`
	Slug      string `gorm:"size:100;not null;uniqueIndex"`
	CreatedAt time.Time
	PostCount int          `gorm:"->"` // populated from query, not stored in DB
	PostItems []PostSeries `gorm:"foreignKey:SeriesID"`
}

// SeriesModel wraps GORM database access for series.
type SeriesModel struct {
	DB *gorm.DB
}

// All returns all series with their published post counts.
func (m *SeriesModel) All() ([]Series, error) {
	var list []Series
	// Get series with post count from post_series join table
	if err := m.DB.Model(&Series{}).
		Select("series.*, COUNT(ps.post_id) AS post_count").
		Joins("LEFT JOIN post_series ps ON series.id = ps.series_id").
		Group("series.id").
		Order("series.name").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetByID returns a single series by ID.
func (m *SeriesModel) GetByID(id int) (*Series, error) {
	var s Series
	if err := m.DB.First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a new series.
func (m *SeriesModel) Create(name, slug string) (int64, error) {
	s := Series{Name: name, Slug: slug}
	if err := m.DB.Create(&s).Error; err != nil {
		return 0, err
	}
	return int64(s.ID), nil
}

// Update renames a series.
func (m *SeriesModel) Update(id int, name, slug string) error {
	return m.DB.Model(&Series{ID: id}).Updates(map[string]any{
		"name": name,
		"slug": slug,
	}).Error
}

// Delete removes a series (cascade deletes post_series entries).
func (m *SeriesModel) Delete(id int) error {
	// Clear join table entries first
	m.DB.Where("series_id = ?", id).Delete(&PostSeries{})
	return m.DB.Delete(&Series{ID: id}).Error
}

// NextSortOrder returns total+1 for the given series.
func (m *SeriesModel) NextSortOrder(seriesID int) (int, error) {
	var count int64
	if err := m.DB.Model(&PostSeries{}).Where("series_id = ?", seriesID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count) + 1, nil
}

// LinkPost assigns a post to a series with the given sort order.
// Unlinks from any existing series first to enforce one-post-one-series.
func (m *SeriesModel) LinkPost(postID, seriesID, sortOrder int) error {
	if err := m.UnlinkPost(postID); err != nil {
		return err
	}
	return m.DB.Create(&PostSeries{
		PostID:    postID,
		SeriesID:  seriesID,
		SortOrder: sortOrder,
	}).Error
}

// UnlinkPost removes a post from all series.
func (m *SeriesModel) UnlinkPost(postID int) error {
	return m.DB.Where("post_id = ?", postID).Delete(&PostSeries{}).Error
}

// GetPostSeries returns the series assigned to a post (if any).
func (m *SeriesModel) GetPostSeries(postID int) (*Series, int, error) {
	var ps PostSeries
	if err := m.DB.Where("post_id = ?", postID).First(&ps).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	var s Series
	if err := m.DB.First(&s, ps.SeriesID).Error; err != nil {
		return nil, 0, err
	}
	return &s, ps.SortOrder, nil
}

// UpdateSortOrder changes the sort_order for a post within a series.
func (m *SeriesModel) UpdateSortOrder(postID, seriesID, sortOrder int) error {
	return m.DB.Model(&PostSeries{}).
		Where("post_id = ? AND series_id = ?", postID, seriesID).
		Update("sort_order", sortOrder).Error
}

// ListPostsBySeries returns posts in a series ordered by sort_order.
func (m *SeriesModel) ListPostsBySeries(seriesID int) ([]PostCard, error) {
	var posts []Post
	if err := m.DB.
		Preload("Category").
		Preload("Tags").
		Joins("JOIN post_series ps ON posts.id = ps.post_id").
		Where("ps.series_id = ?", seriesID).
		Order("ps.sort_order ASC").
		Find(&posts).Error; err != nil {
		return nil, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = postToCard(&p)
	}
	return cards, nil
}
