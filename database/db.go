package database

import (
	"database/sql"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		slug TEXT NOT NULL UNIQUE
	)`,
	`CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		excerpt TEXT NOT NULL DEFAULT '',
		content_md TEXT NOT NULL,
		content_html TEXT NOT NULL,
		category_id INTEGER,
		is_published INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (category_id) REFERENCES categories(id)
	)`,
	`CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		slug TEXT NOT NULL UNIQUE
	)`,
	`CREATE TABLE IF NOT EXISTS post_tags (
		post_id INTEGER,
		tag_id INTEGER,
		PRIMARY KEY (post_id, tag_id),
		FOREIGN KEY (post_id) REFERENCES posts(id),
		FOREIGN KEY (tag_id) REFERENCES tags(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_posts_slug ON posts(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_posts_category ON posts(category_id)`,
	`CREATE INDEX IF NOT EXISTS idx_posts_created ON posts(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_categories_slug ON categories(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_tags_slug ON tags(slug)`,
	`PRAGMA journal_mode=WAL`,
	`PRAGMA foreign_keys=ON`,
	// v2: 文章缩略图
	`ALTER TABLE posts ADD COLUMN thumbnail_url TEXT DEFAULT ''`,
	// v3: 上传资源管理
	`CREATE TABLE IF NOT EXISTS resources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		filename TEXT NOT NULL,
		url TEXT NOT NULL,
		file_size INTEGER NOT NULL DEFAULT 0,
		mime_type TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE SET NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_resources_post_id ON resources(post_id)`,
}

func Init(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// 忽略 "duplicate column" 错误（重复迁移）
			if strings.Contains(err.Error(), "duplicate column") {
				continue
			}
			return nil, err
		}
	}

	log.Println("Database initialized successfully")
	return db, nil
}
