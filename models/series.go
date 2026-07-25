package models

import (
	"database/sql"
	"time"
)

// Series represents a named collection of posts with ordering.
type Series struct {
	ID        int
	Name      string
	Slug      string
	PostCount int
	CreatedAt time.Time
}

// SeriesPost is a post within a series, including its sort order.
type SeriesPost struct {
	PostID    int
	SeriesID  int
	SortOrder int
}

// SeriesModel wraps database access for series.
type SeriesModel struct {
	DB *sql.DB
}

// All returns all series with their published post counts.
func (m *SeriesModel) All() ([]Series, error) {
	rows, err := m.DB.Query(`
		SELECT s.id, s.name, s.slug, COUNT(ps.post_id) AS post_count, s.created_at
		FROM series s
		LEFT JOIN post_series ps ON s.id = ps.series_id
		GROUP BY s.id
		ORDER BY s.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Series
	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &s.PostCount, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// GetByID returns a single series by ID.
func (m *SeriesModel) GetByID(id int) (*Series, error) {
	var s Series
	err := m.DB.QueryRow(`SELECT id, name, slug FROM series WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Slug)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a new series.
func (m *SeriesModel) Create(name, slug string) (int64, error) {
	result, err := m.DB.Exec(`INSERT INTO series (name, slug) VALUES (?, ?)`, name, slug)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Update renames a series.
func (m *SeriesModel) Update(id int, name, slug string) error {
	_, err := m.DB.Exec(`UPDATE series SET name = ?, slug = ? WHERE id = ?`, name, slug, id)
	return err
}

// Delete removes a series (cascade deletes post_series entries).
func (m *SeriesModel) Delete(id int) error {
	_, err := m.DB.Exec(`DELETE FROM series WHERE id = ?`, id)
	return err
}

// NextSortOrder returns total+1 for the given series, used to auto-fill sort_order.
func (m *SeriesModel) NextSortOrder(seriesID int) int {
	var count int
	m.DB.QueryRow(`SELECT COUNT(*) FROM post_series WHERE series_id = ?`, seriesID).Scan(&count)
	return count + 1
}

// LinkPost assigns a post to a series with the given sort order (INSERT or UPDATE).
func (m *SeriesModel) LinkPost(postID, seriesID, sortOrder int) error {
	_, err := m.DB.Exec(`
		INSERT INTO post_series (post_id, series_id, sort_order) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE series_id = VALUES(series_id), sort_order = VALUES(sort_order)
	`, postID, seriesID, sortOrder)
	return err
}

// UnlinkPost removes a post from all series.
func (m *SeriesModel) UnlinkPost(postID int) error {
	_, err := m.DB.Exec(`DELETE FROM post_series WHERE post_id = ?`, postID)
	return err
}

// GetPostSeries returns the series assigned to a post (if any).
func (m *SeriesModel) GetPostSeries(postID int) (*Series, int, error) {
	var s Series
	var sortOrder int
	err := m.DB.QueryRow(`
		SELECT s.id, s.name, s.slug, ps.sort_order
		FROM post_series ps
		JOIN series s ON ps.series_id = s.id
		WHERE ps.post_id = ?
	`, postID).Scan(&s.ID, &s.Name, &s.Slug, &sortOrder)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return &s, sortOrder, nil
}

// UpdateSortOrder changes the sort_order for a post within a series.
func (m *SeriesModel) UpdateSortOrder(postID, seriesID, sortOrder int) error {
	_, err := m.DB.Exec(`UPDATE post_series SET sort_order = ? WHERE post_id = ? AND series_id = ?`,
		sortOrder, postID, seriesID)
	return err
}

// ListPostsBySeries returns posts in a series ordered by sort_order.
func (m *SeriesModel) ListPostsBySeries(seriesID int) ([]PostCard, error) {
	rows, err := m.DB.Query(`
		SELECT p.id, p.title, p.slug, p.excerpt, p.content_html, p.thumbnail_url, p.publish_at, p.created_at,
			   COALESCE(c.name, '') AS category_name,
			   COALESCE(c.slug, '') AS category_slug,
			   ps.sort_order
		FROM post_series ps
		JOIN posts p ON ps.post_id = p.id
		LEFT JOIN categories c ON p.category_id = c.id
		WHERE ps.series_id = ?
		ORDER BY ps.sort_order ASC
	`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []PostCard
	for rows.Next() {
		var card PostCard
		var sortOrder int
		if err := rows.Scan(&card.ID, &card.Title, &card.Slug, &card.Excerpt, &card.ContentHTML, &card.ThumbnailURL, &card.PublishAt, &card.CreatedAt,
			&card.CategoryName, &card.CategorySlug, &sortOrder); err != nil {
			return nil, err
		}
		card.Tags, _ = (&PostModel{DB: m.DB}).TagsForPost(card.ID)
		cards = append(cards, card)
	}
	return cards, nil
}
