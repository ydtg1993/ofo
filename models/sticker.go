package models

import (
	"database/sql"
	"time"
)

// Sticker represents a public media asset (image/video) that can be used
// anywhere — in article body, as cover image, etc. Stickers are independent
// of posts: deleting a post or its references never deletes a sticker.
type Sticker struct {
	ID        int
	Filename  string
	URL       string
	FileSize  int64
	MimeType  string
	CreatedAt time.Time
}

// StickerModel wraps database access for stickers.
type StickerModel struct {
	DB *sql.DB
}

// Count returns the total number of stickers.
func (m *StickerModel) Count() (int, error) {
	var total int
	err := m.DB.QueryRow("SELECT COUNT(*) FROM stickers").Scan(&total)
	return total, err
}

// ListPaginated returns stickers ordered by newest first with offset/limit.
func (m *StickerModel) ListPaginated(offset, limit int) ([]Sticker, error) {
	rows, err := m.DB.Query(`
		SELECT id, filename, url, file_size, mime_type, created_at
		FROM stickers ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stickers []Sticker
	for rows.Next() {
		var s Sticker
		if err := rows.Scan(&s.ID, &s.Filename, &s.URL, &s.FileSize, &s.MimeType, &s.CreatedAt); err != nil {
			return nil, err
		}
		stickers = append(stickers, s)
	}
	return stickers, rows.Err()
}

// Create inserts a new sticker record.
func (m *StickerModel) Create(filename, url string, fileSize int64, mimeType string) (int64, error) {
	result, err := m.DB.Exec(`
		INSERT INTO stickers (filename, url, file_size, mime_type) VALUES (?, ?, ?, ?)
	`, filename, url, fileSize, mimeType)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID returns a single sticker by its numeric ID.
func (m *StickerModel) GetByID(id int) (*Sticker, error) {
	s := &Sticker{}
	err := m.DB.QueryRow(`
		SELECT id, filename, url, file_size, mime_type, created_at
		FROM stickers WHERE id = ?
	`, id).Scan(&s.ID, &s.Filename, &s.URL, &s.FileSize, &s.MimeType, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Delete removes a sticker record by ID.
func (m *StickerModel) Delete(id int) error {
	_, err := m.DB.Exec(`DELETE FROM stickers WHERE id = ?`, id)
	return err
}
