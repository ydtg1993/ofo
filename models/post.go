package models

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	gopinyin "github.com/mozillazg/go-pinyin"
	"gorm.io/gorm"
)

// ---- GORM Model Structs ----

// Category represents a blog post category (GORM model).
type Category struct {
	ID    int    `gorm:"primaryKey;autoIncrement"`
	Name  string `gorm:"size:100;not null;uniqueIndex"`
	Slug  string `gorm:"size:100;not null;uniqueIndex"`
	Posts []Post `gorm:"foreignKey:CategoryID"`
	Count int    `gorm:"-"` // populated from query, not stored in DB
}

// Post represents a full blog post (GORM model).
type Post struct {
	ID           int        `gorm:"primaryKey;autoIncrement"`
	Title        string     `gorm:"size:255;not null"`
	Slug         string     `gorm:"size:255;not null;uniqueIndex"`
	Excerpt      string     `gorm:"type:text;not null"`
	ContentMD    string     `gorm:"column:content_md;type:mediumtext;not null"`
	ContentHTML  string     `gorm:"column:content_html;type:mediumtext;not null"`
	CategoryID   *int       `gorm:"default:null"`
	Category     *Category  `gorm:"foreignKey:CategoryID"`
	IsPublished  bool       `gorm:"column:is_published;type:int;default:1"`
	ThumbnailURL string     `gorm:"column:thumbnail_url;size:512"`
	PublishAt    *time.Time `gorm:"column:publish_at"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Tags         []Tag        `gorm:"many2many:post_tags"`
	Resources    []Resource   `gorm:"many2many:post_resources"`
	SeriesItems  []PostSeries `gorm:"foreignKey:PostID"`
}

// Tag represents a blog post tag (GORM model).
type Tag struct {
	ID    int    `gorm:"primaryKey;autoIncrement"`
	Name  string `gorm:"size:100;not null;uniqueIndex"`
	Slug  string `gorm:"size:100;not null;uniqueIndex"`
	Posts []Post `gorm:"many2many:post_tags"`
	Count int    `gorm:"-"` // populated from query, not stored in DB
}

// PostSeries is the explicit join table for posts<->series with sort_order.
type PostSeries struct {
	PostID    int `gorm:"primaryKey"`
	SeriesID  int `gorm:"primaryKey"`
	SortOrder int `gorm:"column:sort_order;default:0"`
}

// ---- Lightweight DTO structs (not GORM models) ----

// PostCard is a lightweight post representation for listing pages.
type PostCard struct {
	ID           int
	Title        string
	Slug         string
	Excerpt      string
	ContentHTML  string // full HTML for feed display
	ThumbnailURL string
	// ThumbnailWidth/Height are populated by the API layer from storage metadata.
	ThumbnailWidth  int `json:",omitempty"`
	ThumbnailHeight int `json:",omitempty"`
	CategoryName    string
	CategorySlug    string
	PublishAt       *time.Time
	CreatedAt       time.Time
	Tags            []Tag
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

// ---- PostModel ----

// PostModel wraps GORM database queries for posts.
type PostModel struct {
	DB *gorm.DB
}

// ---- Published Post Queries ----

// ListPublished returns paginated published posts with their categories and tags.
func (m *PostModel) ListPublished(offset, limit int) ([]PostCard, int, error) {
	var total int64
	now := time.Now()
	if err := m.DB.Model(&Post{}).
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []Post
	if err := m.DB.
		Preload("Category").
		Preload("Tags").
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = postToCard(&p)
	}

	return cards, int(total), nil
}

// GetBySlug returns a single post by slug.
func (m *PostModel) GetBySlug(slug string) (*Post, error) {
	var p Post
	now := time.Now()
	if err := m.DB.
		Preload("Category").
		Where("slug = ?", slug).
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByCategory returns posts filtered by category slug.
func (m *PostModel) ListByCategory(slug string, offset, limit int) ([]PostCard, int, error) {
	now := time.Now()
	var total int64
	if err := m.DB.Model(&Post{}).
		Joins("JOIN categories c ON posts.category_id = c.id").
		Where("c.slug = ?", slug).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []Post
	if err := m.DB.
		Preload("Category").
		Preload("Tags").
		Joins("JOIN categories c ON posts.category_id = c.id").
		Where("c.slug = ?", slug).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Order("posts.created_at DESC").
		Offset(offset).Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = postToCard(&p)
	}

	return cards, int(total), nil
}

// ListByTag returns posts filtered by tag slug.
func (m *PostModel) ListByTag(slug string, offset, limit int) ([]PostCard, int, error) {
	now := time.Now()
	var total int64
	if err := m.DB.Model(&Post{}).
		Joins("JOIN post_tags pt ON posts.id = pt.post_id").
		Joins("JOIN tags t ON pt.tag_id = t.id").
		Where("t.slug = ?", slug).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []Post
	if err := m.DB.
		Preload("Category").
		Preload("Tags").
		Joins("JOIN post_tags pt ON posts.id = pt.post_id").
		Joins("JOIN tags t ON pt.tag_id = t.id").
		Where("t.slug = ?", slug).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Order("posts.created_at DESC").
		Offset(offset).Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = postToCard(&p)
	}

	return cards, int(total), nil
}

// TagsForPost returns all tags for a given post ID.
func (m *PostModel) TagsForPost(postID int) ([]Tag, error) {
	var tags []Tag
	if err := m.DB.Model(&Post{ID: postID}).Association("Tags").Find(&tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// AllCategories returns all categories with post counts.
func (m *PostModel) AllCategories() ([]Category, error) {
	var categories []Category
	now := time.Now()
	if err := m.DB.Model(&Category{}).
		Select("categories.*, COUNT(posts.id) AS count").
		Joins("LEFT JOIN posts ON posts.category_id = categories.id AND posts.is_published = ? AND (posts.publish_at IS NULL OR posts.publish_at <= ?)", true, now).
		Group("categories.id").
		Order("categories.name").
		Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

// AllTags returns all tags with post counts.
func (m *PostModel) AllTags() ([]Tag, error) {
	var tags []Tag
	now := time.Now()
	if err := m.DB.Model(&Tag{}).
		Select("tags.*, COUNT(pt.post_id) AS count").
		Joins("LEFT JOIN post_tags pt ON tags.id = pt.tag_id").
		Joins("LEFT JOIN posts p ON pt.post_id = p.id AND p.is_published = ? AND (p.publish_at IS NULL OR p.publish_at <= ?)", true, now).
		Group("tags.id").
		Order("tags.name").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// GetTagByID returns a single tag by numeric ID.
func (m *PostModel) GetTagByID(id int) (*Tag, error) {
	var t Tag
	if err := m.DB.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTag renames a tag.
func (m *PostModel) UpdateTag(id int, name, slug string) error {
	return m.DB.Model(&Tag{ID: id}).Updates(map[string]any{
		"name": name,
		"slug": slug,
	}).Error
}

// DeleteTag removes a tag. Only succeeds if no posts reference it.
func (m *PostModel) DeleteTag(id int) (bool, error) {
	var count int64
	if err := m.DB.Table("post_tags").Where("tag_id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := m.DB.Delete(&Tag{ID: id}).Error; err != nil {
		return false, err
	}
	return true, nil
}

// ListPostsByTagID returns paginated posts for a given tag ID.
func (m *PostModel) ListPostsByTagID(tagID, offset, limit int) ([]PostCard, int, error) {
	now := time.Now()
	var total int64
	if err := m.DB.Model(&Post{}).
		Joins("JOIN post_tags pt ON posts.id = pt.post_id").
		Where("pt.tag_id = ?", tagID).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []Post
	if err := m.DB.
		Preload("Category").
		Preload("Tags").
		Joins("JOIN post_tags pt ON posts.id = pt.post_id").
		Where("pt.tag_id = ?", tagID).
		Where("posts.is_published = ?", true).
		Where("posts.publish_at IS NULL OR posts.publish_at <= ?", now).
		Order("posts.created_at DESC").
		Offset(offset).Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	cards := make([]PostCard, len(posts))
	for i, p := range posts {
		cards[i] = postToCard(&p)
	}
	return cards, int(total), nil
}

// GetCategoryName returns the category name for a post, or empty string.
func (m *PostModel) GetCategoryName(categoryID *int) string {
	if categoryID == nil {
		return ""
	}
	var c Category
	if err := m.DB.First(&c, *categoryID).Error; err != nil {
		return ""
	}
	return c.Name
}

// GetCategorySlug returns the category slug for a post, or empty string.
func (m *PostModel) GetCategorySlug(categoryID *int) string {
	if categoryID == nil {
		return ""
	}
	var c Category
	if err := m.DB.First(&c, *categoryID).Error; err != nil {
		return ""
	}
	return c.Slug
}

// RecentPosts returns the most recent n published posts for RSS.
func (m *PostModel) RecentPosts(n int) ([]Post, error) {
	now := time.Now()
	var posts []Post
	if err := m.DB.
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		Order("created_at DESC").
		Limit(n).
		Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// ---- Admin CRUD Methods ----

// ListAll returns all posts (including drafts) for the admin dashboard.
func (m *PostModel) ListAll() ([]Post, error) {
	var posts []Post
	if err := m.DB.Order("created_at DESC").Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// CountAll returns the total number of posts (including drafts).
func (m *PostModel) CountAll() (int, error) {
	var total int64
	err := m.DB.Model(&Post{}).Count(&total).Error
	return int(total), err
}

// ListAllPaginated returns posts (including drafts) with offset/limit.
func (m *PostModel) ListAllPaginated(offset, limit int) ([]Post, error) {
	var posts []Post
	if err := m.DB.Order("created_at DESC").Offset(offset).Limit(limit).Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// GetByID returns a post by its numeric ID.
func (m *PostModel) GetByID(id int) (*Post, error) {
	var p Post
	if err := m.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// Create inserts a new post and returns its ID.
func (m *PostModel) Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL string, categoryID *int, published bool, publishAt *time.Time, createdAt time.Time, tagIDs []int) (int64, error) {
	p := Post{
		Title:        title,
		Slug:         slug,
		Excerpt:      excerpt,
		ContentMD:    contentMD,
		ContentHTML:  contentHTML,
		CategoryID:   categoryID,
		IsPublished:  published,
		ThumbnailURL: thumbnailURL,
		PublishAt:    publishAt,
		CreatedAt:    createdAt,
	}

	if err := m.DB.Create(&p).Error; err != nil {
		return 0, err
	}

	// Link tags
	if len(tagIDs) > 0 {
		var tags []Tag
		m.DB.Where("id IN ?", tagIDs).Find(&tags)
		m.DB.Model(&p).Association("Tags").Replace(tags)
	}

	return int64(p.ID), nil
}

// Update modifies an existing post.
func (m *PostModel) Update(id int, title, slug, contentMD, contentHTML, excerpt, thumbnailURL string, categoryID *int, published bool, publishAt *time.Time, createdAt time.Time, tagIDs []int) error {
	p := Post{
		ID:           id,
		Title:        title,
		Slug:         slug,
		Excerpt:      excerpt,
		ContentMD:    contentMD,
		ContentHTML:  contentHTML,
		CategoryID:   categoryID,
		IsPublished:  published,
		ThumbnailURL: thumbnailURL,
		PublishAt:    publishAt,
		CreatedAt:    createdAt,
	}

	// Save updates all fields; GORM uses zero-value checks so use Updates for full control
	if err := m.DB.Model(&Post{ID: id}).Updates(map[string]any{
		"title":         p.Title,
		"slug":          p.Slug,
		"excerpt":       p.Excerpt,
		"content_md":    p.ContentMD,
		"content_html":  p.ContentHTML,
		"category_id":   p.CategoryID,
		"is_published":  p.IsPublished,
		"thumbnail_url": p.ThumbnailURL,
		"publish_at":    p.PublishAt,
		"created_at":    p.CreatedAt,
		"updated_at":    time.Now(),
	}).Error; err != nil {
		return err
	}

	// Re-link tags
	if tagIDs != nil {
		var tags []Tag
		m.DB.Where("id IN ?", tagIDs).Find(&tags)
		m.DB.Model(&Post{ID: id}).Association("Tags").Replace(tags)
	}

	return nil
}

// Delete removes a post by ID (cascade deletes post_tags, post_resources, post_series).
func (m *PostModel) Delete(id int) error {
	p := Post{ID: id}
	// Clear associations to avoid constraint issues
	m.DB.Model(&p).Association("Tags").Clear()
	m.DB.Model(&p).Association("Resources").Clear()
	m.DB.Where("post_id = ?", id).Delete(&PostSeries{})
	return m.DB.Delete(&p).Error
}

// ---- Category CRUD ----

// CreateCategory creates a new category.
func (m *PostModel) CreateCategory(name, slug string) error {
	return m.DB.Create(&Category{Name: name, Slug: slug}).Error
}

// UpdateCategory updates a category's name and slug.
func (m *PostModel) UpdateCategory(id int, name, slug string) error {
	return m.DB.Model(&Category{ID: id}).Updates(map[string]any{
		"name": name,
		"slug": slug,
	}).Error
}

// DeleteCategory removes a category by ID (unlinks posts first).
func (m *PostModel) DeleteCategory(id int) error {
	// Unlink posts from this category
	m.DB.Model(&Post{}).Where("category_id = ?", id).Update("category_id", nil)
	return m.DB.Delete(&Category{ID: id}).Error
}

// AllCategoriesSimple returns categories without counts.
func (m *PostModel) AllCategoriesSimple() ([]Category, error) {
	var cats []Category
	if err := m.DB.Order("name").Find(&cats).Error; err != nil {
		return nil, err
	}
	return cats, nil
}

// AllTagsSimple returns all tags without counts.
func (m *PostModel) AllTagsSimple() ([]Tag, error) {
	var tags []Tag
	if err := m.DB.Order("name").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// FirstOrCreateTag finds a tag by name, or creates one with a generated slug.
func (m *PostModel) FirstOrCreateTag(name string) (Tag, error) {
	slug := slugify(name)
	var t Tag
	// GORM's FirstOrCreate with Attrs: only assigns attrs if not found
	if err := m.DB.Where(Tag{Slug: slug}).Attrs(Tag{Name: name}).FirstOrCreate(&t).Error; err != nil {
		return Tag{}, err
	}
	return t, nil
}

// ---- Adjacent Posts ----

// GetAdjacentPosts returns the previous and next published posts for navigation.
func (m *PostModel) GetAdjacentPosts(currentSlug string) (*PostCard, *PostCard, error) {
	var current Post
	now := time.Now()
	if err := m.DB.Where("slug = ?", currentSlug).
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		First(&current).Error; err != nil {
		return nil, nil, err
	}

	var prev, next *PostCard

	// Previous post (older)
	var prevPost Post
	if err := m.DB.Preload("Category").
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		Where("created_at < ?", current.CreatedAt).
		Order("created_at DESC").
		First(&prevPost).Error; err == nil {
		pc := postToCard(&prevPost)
		prev = &pc
	}

	// Next post (newer)
	var nextPost Post
	if err := m.DB.Preload("Category").
		Where("is_published = ?", true).
		Where("publish_at IS NULL OR publish_at <= ?", now).
		Where("created_at > ?", current.CreatedAt).
		Order("created_at ASC").
		First(&nextPost).Error; err == nil {
		nc := postToCard(&nextPost)
		next = &nc
	}

	return prev, next, nil
}

// ---- Helper functions ----

// postToCard converts a Post to a PostCard, extracting category info.
func postToCard(p *Post) PostCard {
	card := PostCard{
		ID:           p.ID,
		Title:        p.Title,
		Slug:         p.Slug,
		Excerpt:      p.Excerpt,
		ContentHTML:  p.ContentHTML,
		ThumbnailURL: p.ThumbnailURL,
		CategoryName: "",
		CategorySlug: "",
		PublishAt:    p.PublishAt,
		CreatedAt:    p.CreatedAt,
		Tags:         p.Tags,
	}
	if p.Category != nil {
		card.CategoryName = p.Category.Name
		card.CategorySlug = p.Category.Slug
	}
	return card
}

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = chineseToPinyin(s)

	result := ""
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result += strings.ToLower(string(r))
		}
	}
	if result == "" {
		result = fmt.Sprintf("tag%d", time.Now().Unix())
	}
	return result
}

// chineseToPinyin converts Chinese characters to pinyin (lowercase, no tone marks).
func chineseToPinyin(s string) string {
	args := gopinyin.NewArgs()
	args.Style = gopinyin.Normal
	var b strings.Builder
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			py := gopinyin.SinglePinyin(r, args)
			if len(py) > 0 {
				b.WriteString(py[0])
			}
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
