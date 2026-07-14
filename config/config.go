package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port          string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	Title         string
	Author        string
	BaseURL       string
	AdminPassword string
	SeedDB        bool
	AssetVersion  string
	// SEO
	Keywords    string
	BaiduVerify string
	Verify360   string
	SogouVerify string
}

// DSN returns the MariaDB/MySQL data source name.
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// LoadDotenv reads KEY=VALUE pairs from .env in baseDir and exports them as
// environment variables.  Lines starting with # are comments; blank lines are
// skipped.  Values may be quoted with single or double quotes.  This is called
// before Load() so Load() sees the env vars.
func LoadDotenv(baseDir string) {
	path := filepath.Join(baseDir, ".env")
	f, err := os.Open(path)
	if err != nil {
		return // no .env file — ok
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		// Only set if not already set in environment (env var takes priority)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DBHost:        getEnv("DB_HOST", "127.0.0.1"),
		DBPort:        getEnv("DB_PORT", "3306"),
		DBUser:        getEnv("DB_USER", "root"),
		DBPassword:    getEnv("DB_PASSWORD", ""),
		DBName:        getEnv("DB_NAME", "ofo"),
		Title:         getEnv("BLOG_TITLE", "骑自行车"),
		Author:        getEnv("BLOG_AUTHOR", "青头儿包"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
		SeedDB:        getEnvBool("SEED_DB", true),
		AssetVersion:  getEnv("ASSET_VERSION", "1"),
		Keywords:      getEnv("BLOG_KEYWORDS", "搞笑图片,趣味短片,奇闻趣事,搞笑视频"),
		BaiduVerify:   getEnv("BAIDU_VERIFY", ""),
		Verify360:     getEnv("VERIFY_360", ""),
		SogouVerify:   getEnv("SOGOU_VERIFY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}
