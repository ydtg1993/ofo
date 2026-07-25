package admin

import (
	"net/http"

	"ofo/middleware"

	"github.com/gin-gonic/gin"
)

// ---- Login ----

func (a *AdminHandler) AdminLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin_login.html", gin.H{
		"Cfg":   a.Cfg,
		"Error": "",
	})
}

func (a *AdminHandler) AdminLogin(c *gin.Context) {
	password := c.PostForm("password")
	if password != a.Cfg.AdminPassword {
		c.HTML(http.StatusUnauthorized, "admin_login.html", gin.H{
			"Cfg":   a.Cfg,
			"Error": "密码错误",
		})
		return
	}
	middleware.SetAdminCookie(c, a.Cfg.AdminPassword)
	c.Redirect(http.StatusFound, "/admin")
}

// ---- Logout ----

func (a *AdminHandler) AdminLogout(c *gin.Context) {
	middleware.ClearAdminCookie(c)
	c.Redirect(http.StatusFound, "/admin/login")
}
