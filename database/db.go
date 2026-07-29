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

	// Auto-migrate all GORM models
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
