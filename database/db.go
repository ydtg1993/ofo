package database

import (
	"database/sql"
	"strings"

	"ofo/logger"

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
	// 资源复用的多对多关联表（替代 resources.post_id）
	`CREATE TABLE IF NOT EXISTS post_resources (
		post_id INT NOT NULL,
		resource_id INT NOT NULL,
		PRIMARY KEY (post_id, resource_id),
		FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
		FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	// 定时发布：NULL = 立即发布
	`ALTER TABLE posts ADD COLUMN publish_at DATETIME DEFAULT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_posts_publish_at ON posts(publish_at)`,
	`ALTER TABLE resources ADD COLUMN storage VARCHAR(16) NOT NULL DEFAULT 'local'`,
	// 系列管理
	`CREATE TABLE IF NOT EXISTS series (
			id INT NOT NULL AUTO_INCREMENT,
			name VARCHAR(100) NOT NULL,
			slug VARCHAR(100) NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS post_series (
			post_id INT NOT NULL,
			series_id INT NOT NULL,
			sort_order INT NOT NULL DEFAULT 0,
			PRIMARY KEY (post_id, series_id),
			FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
			FOREIGN KEY (series_id) REFERENCES series(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
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
				strings.Contains(err.Error(), "already exists") ||
				strings.Contains(err.Error(), "Can't DROP") ||
				strings.Contains(err.Error(), "Cannot drop") ||
				strings.Contains(err.Error(), "check that column") ||
				strings.Contains(err.Error(), "doesn't exist") {
				logger.Warn("skipping duplicate migration", "err", err)
				continue
			}
			return nil, err
		}
	}

	// 动态处理 resources.post_id → post_resources 迁移
	var colExists int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'resources'
		AND COLUMN_NAME = 'post_id'
	`).Scan(&colExists); err == nil && colExists > 0 {
		// 1. 迁移历史数据到关联表
		if _, err := db.Exec(`
			INSERT IGNORE INTO post_resources (post_id, resource_id)
			SELECT post_id, id FROM resources WHERE post_id IS NOT NULL
		`); err != nil {
			logger.Warn("failed to migrate resource post_id data", "err", err)
		}

		// 2. 删除 FK（名称可能因环境而异）
		var fkName string
		if err := db.QueryRow(`
			SELECT CONSTRAINT_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'resources'
			AND COLUMN_NAME = 'post_id' AND REFERENCED_TABLE_NAME IS NOT NULL
			LIMIT 1
		`).Scan(&fkName); err == nil && fkName != "" {
			if _, err := db.Exec("ALTER TABLE resources DROP FOREIGN KEY `" + fkName + "`"); err != nil {
				logger.Warn("failed to drop FK", "fk", fkName, "err", err)
			}
		}

		// 3. 删除列
		if _, err := db.Exec("ALTER TABLE resources DROP COLUMN post_id"); err != nil {
			logger.Warn("failed to drop post_id column", "err", err)
		} else {
			logger.Info("dropped resources.post_id column")
		}
	}

	logger.Info("Database initialized successfully")
	return db, nil
}
