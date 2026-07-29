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
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Warn),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}

	// 一次性清理：删除旧有的外键约束（后续由应用层逻辑保证数据完整性）
	dropAllForeignKeys(db,
		"posts", "post_tags", "post_resources", "post_series", "resources",
	)

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
