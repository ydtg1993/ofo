package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port          string
	DBPath        string
	Title         string
	Author        string
	BaseURL       string
	AdminPassword string
	// SEO
	Keywords    string
	BaiduVerify string
	Verify360   string
	SogouVerify string
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
		DBPath:        getEnv("DB_PATH", "db/log.db"),
		Title:         getEnv("BLOG_TITLE", "骑自行车"),
		Author:        getEnv("BLOG_AUTHOR", "青头儿包"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:8080"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
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
