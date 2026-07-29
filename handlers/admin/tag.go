package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---- Tag Management ----

func (a *AdminHandler) AdminCreateTagAjax(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	tag, err := a.PostModel.FirstOrCreateTag(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tag"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": tag.ID, "name": tag.Name, "slug": tag.Slug})
}

func (a *AdminHandler) AdminTags(c *gin.Context) {
	tags, _ := a.PostModel.AllTags()
	c.HTML(http.StatusOK, "admin_tags.html", AdminPageData{
		Title: "Tag Management", Cfg: a.Cfg, Tags: tags, ShowTags: true,
	})
}

func (a *AdminHandler) AdminTagPosts(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	tag, err := a.PostModel.GetTagByID(id)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/tags")
		return
	}
	postCards, total, _ := a.PostModel.ListPostsByTagID(id, 0, 50)
	c.HTML(http.StatusOK, "admin_tag_posts.html", AdminPageData{
		Title: "Tag: " + tag.Name, Cfg: a.Cfg, Tag: tag, PostCards: postCards, TotalPosts: total, ShowTagPosts: true,
	})
}

func (a *AdminHandler) AdminUpdateTag(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	name := strings.TrimSpace(c.PostForm("name"))
	if name != "" {
		a.PostModel.UpdateTag(id, name, slugifyStr(name))
	}
	c.Redirect(http.StatusFound, "/admin/tags")
}

func (a *AdminHandler) AdminDeleteTag(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.PostModel.DeleteTag(id)
	c.Redirect(http.StatusFound, "/admin/tags")
}
