package admin

import (
	"net/http"
	"strconv"
	"strings"

	"ofo/logger"

	"github.com/gin-gonic/gin"
)

// ---- Category Management ----

func (a *AdminHandler) AdminCategories(c *gin.Context) {
	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for management", "err", err)
	}

	c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
		Title:          "Category Management",
		Cfg:            a.Cfg,
		Categories:     categories,
		ShowCategories: true,
	})
}

func (a *AdminHandler) AdminCreateCategory(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}
	if name == "" {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	if err := a.PostModel.CreateCategory(name, slug); err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            a.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "创建分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (a *AdminHandler) AdminUpdateCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	if slug == "" {
		slug = slugifyStr(name)
	}

	if err := a.PostModel.UpdateCategory(id, name, slug); err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            a.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "更新分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}

func (a *AdminHandler) AdminDeleteCategory(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := a.PostModel.DeleteCategory(id); err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_categories.html", AdminPageData{
			Title:          "Category Management",
			Cfg:            a.Cfg,
			Categories:     categories,
			ShowCategories: true,
			Error:          "删除分类失败：" + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/categories")
}
