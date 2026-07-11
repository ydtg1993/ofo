package main

import (
	"log"
	"os"
	"path/filepath"

	"ofo/config"
	"ofo/database"
	"ofo/handlers"
	"ofo/models"
	"ofo/router"
)

func main() {
	// ---- 确定项目根目录（使用当前工作目录）----
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}
	log.Printf("Base directory: %s", baseDir)

	// ---- 配置 ----
	cfg := config.Load()

	// ---- 确保必要目录存在 ----
	dbPath := filepath.Join(baseDir, cfg.DBPath)
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create database directory: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "static", "uploads"), 0755); err != nil {
		log.Fatalf("Failed to create uploads directory: %v", err)
	}
	db, err := database.Init(dbPath)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer db.Close()

	// ---- 种子数据 ----
	if err := database.Seed(db); err != nil {
		log.Fatalf("Failed to seed data: %v", err)
	}

	// ---- 依赖组装 ----
	postModel := &models.PostModel{DB: db}
	h := &handlers.Handler{
		PostModel: postModel,
		Cfg:       cfg,
	}

	// ---- 路由 & 中间件 & 启动 ----
	r := router.Setup(cfg, h, baseDir)

	log.Printf("Blog running at %s", cfg.BaseURL)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
