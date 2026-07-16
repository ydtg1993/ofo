package main

import (
	"os"
	"path/filepath"

	"ofo/config"
	"ofo/database"
	"ofo/handlers"
	"ofo/logger"
	"ofo/models"
	"ofo/router"
	"ofo/storage"
)

func main() {
	// ---- 确定项目根目录（使用当前工作目录）----
	baseDir, err := os.Getwd()
	if err != nil {
		logger.Error("Failed to get working directory", "err", err)
		os.Exit(1)
	}
	logger.Info("Base directory", "path", baseDir)

	// ---- 加载 .env 文件（如果存在）----
	config.LoadDotenv(baseDir)

	// ---- 配置 ----
	cfg := config.Load()

	// ---- 初始化日志系统 ----
	if err := logger.Init(cfg.LogLevel, cfg.LogDir); err != nil {
		// Fall back to stderr if logger init itself fails
		println("Failed to init logger:", err.Error())
		os.Exit(1)
	}
	defer logger.Close()

	// ---- 确保必要目录存在（仅本地模式）----
	if cfg.StorageBackend == "local" {
		if err := os.MkdirAll(filepath.Join(baseDir, "static", "uploads"), 0755); err != nil {
			logger.Error("Failed to create uploads directory", "err", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Join(baseDir, "static", "stickers"), 0755); err != nil {
			logger.Error("Failed to create stickers directory", "err", err)
			os.Exit(1)
		}
	}

	db, err := database.Init(cfg.DSN())
	if err != nil {
		logger.Error("Failed to init database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// ---- 种子数据 ----
	if cfg.SeedDB {
		if err := database.Seed(db); err != nil {
			logger.Error("Failed to seed data", "err", err)
			os.Exit(1)
		}
	} else {
		logger.Info("SeedDB is disabled, skipping seed data")
	}

	// ---- 依赖组装 ----
	postModel := &models.PostModel{DB: db}
	resourceModel := &models.ResourceModel{DB: db}
	stickerModel := &models.StickerModel{DB: db}

	// ---- 初始化存储后端 ----
	var store storage.Storage
	switch cfg.StorageBackend {
	case "qiniu":
		s, err := storage.NewQiniuStorage(cfg.QiniuAccessKey, cfg.QiniuSecretKey,
			cfg.QiniuBucket, cfg.QiniuDomain)
		if err != nil {
			logger.Error("Failed to init Qiniu storage", "err", err)
			os.Exit(1)
		}
		store = s
		logger.Info("Storage backend", "backend", "qiniu")
	default:
		store = storage.NewLocalStorage(filepath.Join(baseDir, "static"))
		logger.Info("Storage backend", "backend", "local")

		// 启动时扫描已有上传文件，补录到资源表（仅本地模式）
		uploadsDir := filepath.Join(baseDir, "static", "uploads")
		if n, err := resourceModel.ScanDiskAndRecord(uploadsDir); err != nil {
			logger.Warn("Failed to scan uploads", "err", err)
		} else if n > 0 {
			logger.Info("Recorded previously untracked upload files", "count", n)
		}
	}

	h := &handlers.Handler{
		PostModel:     postModel,
		ResourceModel: resourceModel,
		StickerModel:  stickerModel,
		Cfg:           cfg,
		BaseDir:       baseDir,
		Storage:       store,
	}

	// ---- 路由 & 中间件 & 启动 ----
	r := router.Setup(cfg, h, baseDir)

	logger.Info("Blog starting", "url", cfg.BaseURL, "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		logger.Error("Failed to start server", "err", err)
		os.Exit(1)
	}
}
