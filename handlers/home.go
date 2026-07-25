package handlers

import (
	"net/http"
	"strings"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

// maxHomePosts 首页/API 可加载的文章总数上限。
const maxHomePosts = 200

func (h *Handler) Home(c *gin.Context) {
	// 首次加载 10 篇文章（按创建时间倒序），后续通过 AJAX 无限滚动加载，总数上限 200。
	posts, total, err := h.PostModel.ListPublished(0, 10)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list published posts", "err", err)
		c.HTML(http.StatusInternalServerError, "home.html", PageData{
			Title: "Error", Cfg: h.Cfg, Is404: true,
		})
		return
	}

	// 总数封顶 200
	if total > maxHomePosts {
		total = maxHomePosts
	}

	categories, err := h.PostModel.AllCategories()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for home", "err", err)
	}
	tags, err := h.PostModel.AllTags()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for home", "err", err)
	}

	pd := PageData{
		Title:        h.Cfg.Title,
		Description:  "蹬车摸鱼 — 下班蹬两圈，上班摸一阵。30秒速览、3分钟摸鱼、午休放松，为打工人量身定制的轻娱乐内容。",
		Keywords:     h.Cfg.Keywords,
		CanonicalURL: h.Cfg.BaseURL,
		Cfg:          h.Cfg,
		Categories:   categories,
		Tags:         tags,
		Posts:        posts,
		TotalPosts:   total,
		HasMore:      total > 10,
		CurrentPath:  "/",
		IsHome:       true,
	}

	if isMobileDevice(c.GetHeader("User-Agent")) {
		c.HTML(http.StatusOK, "mobile_home", pd)
	} else {
		c.HTML(http.StatusOK, "home.html", pd)
	}
}

// isMobileDevice 通过 UA 判断是否为手机设备。
func isMobileDevice(ua string) bool {
	ua = strings.ToLower(ua)
	mobileKeywords := []string{"mobile", "android", "iphone", "ipod", "blackberry", "windows phone", "opera mini"}
	for _, kw := range mobileKeywords {
		if strings.Contains(ua, kw) {
			return true
		}
	}
	return false
}

// Fullscreen 全屏刷屏模式，加载全部文章，每条占满视口。
func (h *Handler) Fullscreen(c *gin.Context) {
	posts, _, err := h.PostModel.ListPublished(0, 50)
	if err != nil {
		c.HTML(500, "404.html", PageData{Title: "Error", Cfg: h.Cfg, Is404: true})
		return
	}

	c.HTML(200, "fullscreen.html", PageData{
		Title:        "刷屏 — " + h.Cfg.Title,
		Description:  "全屏刷屏浏览全部内容",
		Keywords:     h.Cfg.Keywords,
		CanonicalURL: h.Cfg.BaseURL + "/fullscreen",
		Cfg:          h.Cfg,
		Posts:        posts,
		CurrentPath:  "/fullscreen",
	})
}
