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

	// ---- 加载 .env 文件（如果存在）----
	config.LoadDotenv(baseDir)

	// ---- 配置 ----
	cfg := config.Load()

	// ---- 确保必要目录存在 ----
	if err := os.MkdirAll(filepath.Join(baseDir, "static", "uploads"), 0755); err != nil {
		log.Fatalf("Failed to create uploads directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "static", "stickers"), 0755); err != nil {
		log.Fatalf("Failed to create stickers directory: %v", err)
	}

	db, err := database.Init(cfg.DSN())
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer db.Close()

	// ---- 种子数据 ----
	if cfg.SeedDB {
		if err := database.Seed(db); err != nil {
			log.Fatalf("Failed to seed data: %v", err)
		}
	} else {
		log.Println("SeedDB is disabled, skipping seed data")
	}

	// ---- 依赖组装 ----
	postModel := &models.PostModel{DB: db}
	resourceModel := &models.ResourceModel{DB: db}
	stickerModel := &models.StickerModel{DB: db}

	// 启动时扫描已有上传文件，补录到资源表（幂等安全）
	uploadsDir := filepath.Join(baseDir, "static", "uploads")
	if n, err := resourceModel.ScanDiskAndRecord(uploadsDir); err != nil {
		log.Printf("Warning: failed to scan uploads: %v", err)
	} else if n > 0 {
		log.Printf("Recorded %d previously untracked upload file(s)", n)
	}

	h := &handlers.Handler{
		PostModel:     postModel,
		ResourceModel: resourceModel,
		StickerModel:  stickerModel,
		Cfg:           cfg,
		BaseDir:       baseDir,
	}

	// ---- 路由 & 中间件 & 启动 ----
	r := router.Setup(cfg, h, baseDir)

	log.Printf("Blog running at %s", cfg.BaseURL)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
