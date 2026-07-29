package database

import (
	"ofo/logger"
	"ofo/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Init opens the database, runs auto-migration, and returns a *gorm.DB.
func Init(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}

	migrateColumns(db)

	if err := db.AutoMigrate(
		&models.Category{},
		&models.Tag{},
		&models.Post{},
		&models.Resource{},
		&models.Series{},
		&models.PostSeries{},
	); err != nil {
		return nil, err
	}

	logger.Info("Database initialized successfully")
	return db, nil
}

// migrateColumns ensures existing INT columns match GORM's BIGINT type mapping
// (Go's int is 64-bit on 64-bit platforms). GORM cannot ALTER a column that
// is part of a FK constraint, so we widen the columns ourselves before
// GORM's AutoMigrate re-creates the constraints.
func migrateColumns(db *gorm.DB) {
	// Drop every FK on the tables we're about to touch
	dropAllForeignKeys(db, "posts", "post_tags", "post_resources", "post_series")

	// Widen PK columns and FK columns from INT to BIGINT so GORM's schema matches.
	// Each ALTER is a no-op if the column is already BIGINT.
	alters := []string{
		// Primary keys
		"ALTER TABLE `categories` MODIFY COLUMN `id` BIGINT AUTO_INCREMENT",
		"ALTER TABLE `posts` MODIFY COLUMN `id` BIGINT AUTO_INCREMENT",
		"ALTER TABLE `tags` MODIFY COLUMN `id` BIGINT AUTO_INCREMENT",
		"ALTER TABLE `resources` MODIFY COLUMN `id` BIGINT AUTO_INCREMENT",
		"ALTER TABLE `series` MODIFY COLUMN `id` BIGINT AUTO_INCREMENT",
		// Foreign keys
		"ALTER TABLE `posts` MODIFY COLUMN `category_id` BIGINT DEFAULT NULL",
		// Join table FK columns (these may not exist yet; ignore errors)
		"ALTER TABLE `post_tags` MODIFY COLUMN `post_id` BIGINT NOT NULL",
		"ALTER TABLE `post_tags` MODIFY COLUMN `tag_id` BIGINT NOT NULL",
		"ALTER TABLE `post_resources` MODIFY COLUMN `post_id` BIGINT NOT NULL",
		"ALTER TABLE `post_resources` MODIFY COLUMN `resource_id` BIGINT NOT NULL",
		"ALTER TABLE `post_series` MODIFY COLUMN `post_id` BIGINT NOT NULL",
		"ALTER TABLE `post_series` MODIFY COLUMN `series_id` BIGINT NOT NULL",
	}
	for _, sql := range alters {
		if err := db.Exec(sql).Error; err != nil {
			// Column may already be BIGINT or the table/column may not exist yet.
			// Both are harmless — AutoMigrate will handle table creation.
			logger.Info("pre-migration ALTER skipped (harmless)", "sql", sql, "err", err)
		}
	}
}

// dropAllForeignKeys drops every FK constraint on the named tables.
func dropAllForeignKeys(db *gorm.DB, tables ...string) {
	for _, table := range tables {
		rows, err := db.Raw(
			`SELECT CONSTRAINT_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
			 AND REFERENCED_TABLE_NAME IS NOT NULL`, table,
		).Rows()
		if err != nil {
			logger.Warn("failed to query FK constraints", "table", table, "err", err)
			continue
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil || name == "" {
				continue
			}
			sql := "ALTER TABLE `" + table + "` DROP FOREIGN KEY `" + name + "`"
			if err := db.Exec(sql).Error; err != nil {
				logger.Warn("failed to drop foreign key", "table", table, "constraint", name, "err", err)
			}
		}
		if err := rows.Err(); err != nil {
			logger.Warn("error iterating FK constraints", "table", table, "err", err)
		}
		rows.Close()
	}
}
