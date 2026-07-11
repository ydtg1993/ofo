package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const cookieName = "admin_token"
const cookieDuration = 24 * time.Hour

func signToken(password string) string {
	h := hmac.New(sha256.New, []byte(password))
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	h.Write([]byte(timestamp))
	return timestamp + "." + hex.EncodeToString(h.Sum(nil))
}

func verifyToken(token, password string) bool {
	dotIdx := -1
	for i, c := range token {
		if c == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx < 0 {
		return false
	}
	timestamp := token[:dotIdx]
	sig := token[dotIdx+1:]

	h := hmac.New(sha256.New, []byte(password))
	h.Write([]byte(timestamp))
	expectedSig := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expectedSig))
}

// AdminAuth is a Gin middleware that requires a valid admin token cookie.
// Unauthenticated requests are redirected to /admin/login.
func AdminAuth(password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(cookieName)
		if err != nil || !verifyToken(token, password) {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// SetAdminCookie sets the admin authentication cookie.
func SetAdminCookie(c *gin.Context, password string) {
	token := signToken(password)
	c.SetCookie(cookieName, token, int(cookieDuration.Seconds()), "/admin", "", false, true)
}

// ClearAdminCookie removes the admin authentication cookie.
func ClearAdminCookie(c *gin.Context) {
	c.SetCookie(cookieName, "", -1, "/admin", "", false, true)
}
