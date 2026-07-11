package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) RSS(c *gin.Context) {
	posts, err := h.PostModel.RecentPosts(20)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error generating feed")
		return
	}

	// Build XML manually for simplicity
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
<title>` + h.Cfg.Title + `</title>
<link>` + h.Cfg.BaseURL + `</link>
<description>搞笑图片、趣味短片、奇闻趣事 —— 内容来源于网络，快乐来源于分享。</description>
<language>zh-CN</language>
<lastBuildDate>` + time.Now().Format(time.RFC1123Z) + `</lastBuildDate>
<atom:link href="` + h.Cfg.BaseURL + `/rss.xml" rel="self" type="application/rss+xml"/>
`

	for _, p := range posts {
		xml += fmt.Sprintf(`<item>
<title>%s</title>
<link>%s/post/%s</link>
<guid>%s/post/%s</guid>
<pubDate>%s</pubDate>
<description><![CDATA[%s]]></description>
</item>
`, escapeXML(p.Title), h.Cfg.BaseURL, p.Slug, h.Cfg.BaseURL, p.Slug,
			p.CreatedAt.Format(time.RFC1123Z), p.Excerpt)
	}

	xml += `</channel>
</rss>`

	c.Header("Content-Type", "application/rss+xml; charset=utf-8")
	c.String(http.StatusOK, xml)
}

func escapeXML(s string) string {
	result := ""
	for _, r := range s {
		switch r {
		case '&':
			result += "&amp;"
		case '<':
			result += "&lt;"
		case '>':
			result += "&gt;"
		case '"':
			result += "&quot;"
		case '\'':
			result += "&apos;"
		default:
			result += string(r)
		}
	}
	return result
}
