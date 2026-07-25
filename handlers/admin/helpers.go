package admin

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ofo/models"

	"github.com/gin-gonic/gin"
	gopinyin "github.com/mozillazg/go-pinyin"
)

// adminPagination builds Pagination info from query params.
func adminPagination(c *gin.Context, total int, perPage int) *models.Pagination {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	return &models.Pagination{
		CurrentPage: page,
		TotalPages:  totalPages,
		PerPage:     perPage,
		TotalPosts:  total,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}
}

func slugifyStr(s string) string {
	// 先将中文转换为拼音，再只保留字母和数字（小写），去掉所有符号和空格
	s = chineseToPinyin(s)

	result := ""
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result += strings.ToLower(string(r))
		}
	}
	if result == "" {
		result = fmt.Sprintf("post%d", time.Now().Unix())
	}
	return result
}

// chineseToPinyin 将字符串中的中文转换为拼音（小写，不含声调），非中文原样保留。
func chineseToPinyin(s string) string {
	args := gopinyin.NewArgs()
	args.Style = gopinyin.Normal // 小写，不带声调
	var b strings.Builder
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff { // CJK Unified Ideographs
			py := gopinyin.SinglePinyin(r, args)
			if len(py) > 0 {
				b.WriteString(py[0])
			}
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func extractExcerptStr(md string, maxLen int) string {
	// Strip markdown roughly
	replacer := strings.NewReplacer("`", "", "#", "", "*", "", "_", "", "[", "", "]", "", "(", "", ")", "")
	clean := replacer.Replace(md)
	// Remove ``` blocks
	for {
		start := strings.Index(clean, "```")
		if start < 0 {
			break
		}
		end := strings.Index(clean[start+3:], "```")
		if end < 0 {
			break
		}
		clean = clean[:start] + clean[start+3+end+3:]
	}
	clean = strings.Join(strings.Fields(clean), " ")

	if len(clean) > maxLen {
		cut := clean[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return clean
}

// resolveTagIDs parses comma-separated tag IDs into []int.
func (a *AdminHandler) resolveTagIDs(tagIDsStr string) ([]int, error) {
	if strings.TrimSpace(tagIDsStr) == "" {
		return nil, nil
	}
	var ids []int
	for _, s := range strings.Split(tagIDsStr, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseTags(tagStr string) []string {
	if strings.TrimSpace(tagStr) == "" {
		return nil
	}
	// 按换行拆分，兼容 \n 和 \r\n
	parts := strings.FieldsFunc(tagStr, func(r rune) bool { return r == '\n' || r == '\r' })
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// parseDateTime parses a form datetime-local string, returns NullTime (NULL if empty).
func parseDateTime(s string) sql.NullTime {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullTime{}
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}
	return sql.NullTime{}
}

// parseDate parses a form date string, defaults to today.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t
		}
	}
	return time.Now()
}
