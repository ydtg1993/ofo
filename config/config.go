package config

import "os"

type Config struct {
	Port          string
	DBPath        string
	Title         string
	Author        string
	BaseURL       string
	AdminPassword string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DBPath:        getEnv("DB_PATH", "db/log.db"),
		Title:         getEnv("BLOG_TITLE", "骑自行车"),
		Author:        getEnv("BLOG_AUTHOR", "青头儿包"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
