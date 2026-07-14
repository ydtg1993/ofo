package database

import (
	"database/sql"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS categories (
		id INT NOT NULL AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL UNIQUE,
		slug VARCHAR(100) NOT NULL UNIQUE,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS posts (
		id INT NOT NULL AUTO_INCREMENT,
		title VARCHAR(255) NOT NULL,
		slug VARCHAR(255) NOT NULL UNIQUE,
		excerpt TEXT NOT NULL,
		content_md MEDIUMTEXT NOT NULL,
		content_html MEDIUMTEXT NOT NULL,
		category_id INT,
		is_published INT DEFAULT 1,
		thumbnail_url VARCHAR(512) DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		FOREIGN KEY (category_id) REFERENCES categories(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS tags (
		id INT NOT NULL AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL UNIQUE,
		slug VARCHAR(100) NOT NULL UNIQUE,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS post_tags (
		post_id INT,
		tag_id INT,
		PRIMARY KEY (post_id, tag_id),
		FOREIGN KEY (post_id) REFERENCES posts(id),
		FOREIGN KEY (tag_id) REFERENCES tags(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE INDEX IF NOT EXISTS idx_posts_slug ON posts(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_posts_category ON posts(category_id)`,
	`CREATE INDEX IF NOT EXISTS idx_posts_created ON posts(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_categories_slug ON categories(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_tags_slug ON tags(slug)`,
	`CREATE TABLE IF NOT EXISTS resources (
		id INT NOT NULL AUTO_INCREMENT,
		post_id INT,
		filename VARCHAR(255) NOT NULL,
		url VARCHAR(512) NOT NULL,
		file_size BIGINT NOT NULL DEFAULT 0,
		mime_type VARCHAR(100) NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE SET NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE INDEX IF NOT EXISTS idx_resources_post_id ON resources(post_id)`,
}

func Init(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// 忽略重复迁移错误
			if strings.Contains(err.Error(), "Duplicate column") ||
				strings.Contains(err.Error(), "Duplicate key") ||
				strings.Contains(err.Error(), "already exists") {
				continue
			}
			return nil, err
		}
	}

	log.Println("Database initialized successfully")
	return db, nil
}
