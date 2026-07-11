package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Post represents a full blog post from the database.
type Post struct {
	ID           int
	Title        string
	Slug         string
	Excerpt      string
	ContentMD    string
	ContentHTML  string
	CategoryID   sql.NullInt64
	IsPublished  bool
	ThumbnailURL string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PostCard is a lightweight post representation for listing pages.
type PostCard struct {
	ID           int
	Title        string
	Slug         string
	Excerpt      string
	ThumbnailURL string
	CategoryName string
	CategorySlug string
	CreatedAt    time.Time
	Tags         []Tag
}

// Category represents a blog post category.
type Category struct {
	ID    int
	Name  string
	Slug  string
	Count int
}

// Tag represents a blog post tag.
type Tag struct {
	ID    int
	Name  string
	Slug  string
	Count int
}

// Pagination holds page navigation info.
type Pagination struct {
	CurrentPage int
	TotalPages  int
	PerPage     int
	TotalPosts  int
	HasPrev     bool
	HasNext     bool
	PrevPage    int
	NextPage    int
}

// PostModel wraps database queries for posts.
type PostModel struct {
	DB *sql.DB
}

// ListPublished returns paginated published posts with their categories and tags.
func (m *PostModel) ListPublished(offset, limit int) ([]PostCard, int, error) {
	total := 0
	if err := m.DB.QueryRow("SELECT COUNT(*) FROM posts WHERE is_published = 1").Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := m.DB.Query(`
		SELECT p.id, p.title, p.slug, p.excerpt, p.thumbnail_url, p.created_at,
			   COALESCE(c.name, '') AS category_name,
			   COALESCE(c.slug, '') AS category_slug
		FROM posts p
		LEFT JOIN categories c ON p.category_id = c.id
		WHERE p.is_published = 1
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cards []PostCard
	for rows.Next() {
		var card PostCard
		if err := rows.Scan(&card.ID, &card.Title, &card.Slug, &card.Excerpt, &card.ThumbnailURL, &card.CreatedAt,
			&card.CategoryName, &card.CategorySlug); err != nil {
			return nil, 0, err
		}

		// Load tags for this post
		card.Tags, _ = m.TagsForPost(card.ID)
		cards = append(cards, card)
	}

	return cards, total, nil
}

// GetBySlug returns a single post by slug.
func (m *PostModel) GetBySlug(slug string) (*Post, error) {
	p := &Post{}
	err := m.DB.QueryRow(`
		SELECT id, title, slug, excerpt, content_md, content_html, category_id, is_published, thumbnail_url, created_at, updated_at
		FROM posts WHERE slug = ? AND is_published = 1
	`, slug).Scan(&p.ID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentMD,
		&p.ContentHTML, &p.CategoryID, &p.IsPublished, &p.ThumbnailURL, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ListByCategory returns posts filtered by category slug.
func (m *PostModel) ListByCategory(slug string, offset, limit int) ([]PostCard, int, error) {
	total := 0
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM posts p
		JOIN categories c ON p.category_id = c.id
		WHERE c.slug = ? AND p.is_published = 1
	`, slug).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := m.DB.Query(`
		SELECT p.id, p.title, p.slug, p.excerpt, p.thumbnail_url, p.created_at,
			   COALESCE(c.name, '') AS category_name,
			   COALESCE(c.slug, '') AS category_slug
		FROM posts p
		JOIN categories c ON p.category_id = c.id
		WHERE c.slug = ? AND p.is_published = 1
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, slug, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cards []PostCard
	for rows.Next() {
		var card PostCard
		if err := rows.Scan(&card.ID, &card.Title, &card.Slug, &card.Excerpt, &card.ThumbnailURL, &card.CreatedAt,
			&card.CategoryName, &card.CategorySlug); err != nil {
			return nil, 0, err
		}
		card.Tags, _ = m.TagsForPost(card.ID)
		cards = append(cards, card)
	}

	return cards, total, nil
}

// ListByTag returns posts filtered by tag slug.
func (m *PostModel) ListByTag(slug string, offset, limit int) ([]PostCard, int, error) {
	total := 0
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE t.slug = ? AND p.is_published = 1
	`, slug).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := m.DB.Query(`
		SELECT p.id, p.title, p.slug, p.excerpt, p.thumbnail_url, p.created_at,
			   COALESCE(c.name, '') AS category_name,
			   COALESCE(c.slug, '') AS category_slug
		FROM posts p
		LEFT JOIN categories c ON p.category_id = c.id
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE t.slug = ? AND p.is_published = 1
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, slug, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cards []PostCard
	for rows.Next() {
		var card PostCard
		if err := rows.Scan(&card.ID, &card.Title, &card.Slug, &card.Excerpt, &card.ThumbnailURL, &card.CreatedAt,
			&card.CategoryName, &card.CategorySlug); err != nil {
			return nil, 0, err
		}
		card.Tags, _ = m.TagsForPost(card.ID)
		cards = append(cards, card)
	}

	return cards, total, nil
}

// TagsForPost returns all tags for a given post ID.
func (m *PostModel) TagsForPost(postID int) ([]Tag, error) {
	rows, err := m.DB.Query(`
		SELECT t.id, t.name, t.slug
		FROM tags t
		JOIN post_tags pt ON t.id = pt.tag_id
		WHERE pt.post_id = ?
		ORDER BY t.name
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Slug); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// AllCategories returns all categories with post counts.
func (m *PostModel) AllCategories() ([]Category, error) {
	rows, err := m.DB.Query(`
		SELECT c.id, c.name, c.slug, COUNT(p.id) AS count
		FROM categories c
		LEFT JOIN posts p ON p.category_id = c.id AND p.is_published = 1
		GROUP BY c.id
		ORDER BY c.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Count); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

// AllTags returns all tags with post counts.
func (m *PostModel) AllTags() ([]Tag, error) {
	rows, err := m.DB.Query(`
		SELECT t.id, t.name, t.slug, COUNT(pt.post_id) AS count
		FROM tags t
		LEFT JOIN post_tags pt ON t.id = pt.tag_id
		LEFT JOIN posts p ON pt.post_id = p.id AND p.is_published = 1
		GROUP BY t.id
		ORDER BY t.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Count); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

// GetCategoryName returns the category name for a post, or empty string.
func (m *PostModel) GetCategoryName(categoryID sql.NullInt64) string {
	if !categoryID.Valid {
		return ""
	}
	var name string
	if err := m.DB.QueryRow("SELECT name FROM categories WHERE id = ?", categoryID.Int64).Scan(&name); err != nil {
		return ""
	}
	return name
}

// GetCategorySlug returns the category slug for a post, or empty string.
func (m *PostModel) GetCategorySlug(categoryID sql.NullInt64) string {
	if !categoryID.Valid {
		return ""
	}
	var slug string
	if err := m.DB.QueryRow("SELECT slug FROM categories WHERE id = ?", categoryID.Int64).Scan(&slug); err != nil {
		return ""
	}
	return slug
}

// RecentPosts returns the most recent n published posts for RSS.
func (m *PostModel) RecentPosts(n int) ([]Post, error) {
	rows, err := m.DB.Query(`
		SELECT id, title, slug, excerpt, content_md, content_html, category_id, is_published, thumbnail_url, created_at, updated_at
		FROM posts WHERE is_published = 1
		ORDER BY created_at DESC LIMIT ?
	`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentMD,
			&p.ContentHTML, &p.CategoryID, &p.IsPublished, &p.ThumbnailURL, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// ---- Admin CRUD Methods ----

// ListAll returns all posts (including drafts) for the admin dashboard.
func (m *PostModel) ListAll() ([]Post, error) {
	rows, err := m.DB.Query(`
		SELECT id, title, slug, excerpt, content_md, content_html, category_id, is_published, thumbnail_url, created_at, updated_at
		FROM posts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentMD,
			&p.ContentHTML, &p.CategoryID, &p.IsPublished, &p.ThumbnailURL, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// GetByID returns a post by its numeric ID.
func (m *PostModel) GetByID(id int) (*Post, error) {
	p := &Post{}
	err := m.DB.QueryRow(`
		SELECT id, title, slug, excerpt, content_md, content_html, category_id, is_published, thumbnail_url, created_at, updated_at
		FROM posts WHERE id = ?
	`, id).Scan(&p.ID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentMD,
		&p.ContentHTML, &p.CategoryID, &p.IsPublished, &p.ThumbnailURL, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Create inserts a new post and returns its ID.
func (m *PostModel) Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL string, categoryID sql.NullInt64, published bool, createdAt time.Time, tagNames []string) (int64, error) {
	pubInt := 0
	if published {
		pubInt = 1
	}

	result, err := m.DB.Exec(`
		INSERT INTO posts (title, slug, excerpt, content_md, content_html, category_id, is_published, thumbnail_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, title, slug, excerpt, contentMD, contentHTML, categoryID, pubInt, thumbnailURL, createdAt)
	if err != nil {
		return 0, err
	}

	postID, _ := result.LastInsertId()

	// Link tags
	for _, tagName := range tagNames {
		if tagName == "" {
			continue
		}
		tagSlug := slugify(tagName)
		m.DB.Exec("INSERT OR IGNORE INTO tags (name, slug) VALUES (?, ?)", tagName, tagSlug)

		var tagID int64
		if err := m.DB.QueryRow("SELECT id FROM tags WHERE slug = ?", tagSlug).Scan(&tagID); err == nil {
			m.DB.Exec("INSERT OR IGNORE INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID)
		}
	}

	return postID, nil
}

// Update modifies an existing post.
func (m *PostModel) Update(id int, title, slug, contentMD, contentHTML, excerpt, thumbnailURL string, categoryID sql.NullInt64, published bool, createdAt time.Time, tagNames []string) error {
	pubInt := 0
	if published {
		pubInt = 1
	}

	_, err := m.DB.Exec(`
		UPDATE posts SET title=?, slug=?, excerpt=?, content_md=?, content_html=?, category_id=?, is_published=?, thumbnail_url=?, created_at=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, title, slug, excerpt, contentMD, contentHTML, categoryID, pubInt, thumbnailURL, createdAt, id)
	if err != nil {
		return err
	}

	// Re-link tags: delete existing, re-insert
	m.DB.Exec("DELETE FROM post_tags WHERE post_id = ?", id)
	for _, tagName := range tagNames {
		if tagName == "" {
			continue
		}
		tagSlug := slugify(tagName)
		m.DB.Exec("INSERT OR IGNORE INTO tags (name, slug) VALUES (?, ?)", tagName, tagSlug)

		var tagID int64
		if err := m.DB.QueryRow("SELECT id FROM tags WHERE slug = ?", tagSlug).Scan(&tagID); err == nil {
			m.DB.Exec("INSERT OR IGNORE INTO post_tags (post_id, tag_id) VALUES (?, ?)", id, tagID)
		}
	}

	return nil
}

// Delete removes a post by ID.
func (m *PostModel) Delete(id int) error {
	m.DB.Exec("DELETE FROM post_tags WHERE post_id = ?", id)
	_, err := m.DB.Exec("DELETE FROM posts WHERE id = ?", id)
	return err
}

// ---- Category CRUD ----

// CreateCategory creates a new category.
func (m *PostModel) CreateCategory(name, slug string) error {
	_, err := m.DB.Exec("INSERT INTO categories (name, slug) VALUES (?, ?)", name, slug)
	return err
}

// UpdateCategory updates a category's name and slug.
func (m *PostModel) UpdateCategory(id int, name, slug string) error {
	_, err := m.DB.Exec("UPDATE categories SET name=?, slug=? WHERE id=?", name, slug, id)
	return err
}

// DeleteCategory removes a category by ID.
func (m *PostModel) DeleteCategory(id int) error {
	// Unlink posts from this category
	m.DB.Exec("UPDATE posts SET category_id = NULL WHERE category_id = ?", id)
	_, err := m.DB.Exec("DELETE FROM categories WHERE id = ?", id)
	return err
}

// AllCategoriesSimple returns categories without counts.
func (m *PostModel) AllCategoriesSimple() ([]Category, error) {
	rows, err := m.DB.Query("SELECT id, name, slug FROM categories ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

// AllTagsSimple returns all tags without counts.
func (m *PostModel) AllTagsSimple() ([]Tag, error) {
	rows, err := m.DB.Query("SELECT id, name, slug FROM tags ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

// ---- Thumbnail Extraction ----

// ExtractThumbnail extracts the first image or video URL from HTML content.
func ExtractThumbnail(html string) string {
	// Try <img> first
	if idx := strings.Index(html, "<img "); idx >= 0 {
		srcStart := strings.Index(html[idx:], "src=\"")
		if srcStart < 0 {
			srcStart = strings.Index(html[idx:], "src='")
		}
		if srcStart >= 0 {
			srcStart += 5 // skip 'src="'
			sub := html[idx+srcStart:]
			srcEnd := strings.IndexAny(sub, "\"'")
			if srcEnd > 0 {
				return sub[:srcEnd]
			}
		}
	}

	// Try <video> / <source>
	if idx := strings.Index(html, "<video "); idx >= 0 {
		srcStart := strings.Index(html[idx:], "src=\"")
		if srcStart < 0 {
			srcStart = strings.Index(html[idx:], "src='")
		}
		if srcStart >= 0 {
			srcStart += 5
			sub := html[idx+srcStart:]
			srcEnd := strings.IndexAny(sub, "\"'")
			if srcEnd > 0 {
				return sub[:srcEnd]
			}
		}
	}

	return ""
}

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	result := ""
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result += strings.ToLower(string(r))
		} else if r == ' ' || r == '-' || r == '_' {
			if len(result) > 0 && result[len(result)-1] != '-' {
				result += "-"
			}
		}
	}
	slug := strings.TrimRight(result, "-")
	if slug == "" {
		slug = fmt.Sprintf("tag-%d", time.Now().Unix())
	}
	return slug
}
