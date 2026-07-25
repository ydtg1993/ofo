package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---- Series Management ----

func (a *AdminHandler) AdminSeries(c *gin.Context) {
	list, _ := a.SeriesModel.All()
	c.HTML(http.StatusOK, "admin_series.html", AdminPageData{
		Title: "Series Management", Cfg: a.Cfg, SeriesList: list, ShowSeries: true,
	})
}

func (a *AdminHandler) AdminSeriesPosts(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	series, err := a.SeriesModel.GetByID(id)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/series")
		return
	}
	postCards, _ := a.SeriesModel.ListPostsBySeries(id)
	c.HTML(http.StatusOK, "admin_series_posts.html", AdminPageData{
		Title: "Series: " + series.Name, Cfg: a.Cfg, Series: series, PostCards: postCards, ShowSeriesPosts: true,
	})
}

func (a *AdminHandler) AdminCreateSeries(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name != "" {
		a.SeriesModel.Create(name, slugifyStr(name))
	}
	c.Redirect(http.StatusFound, "/admin/series")
}

func (a *AdminHandler) AdminUpdateSeries(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	name := strings.TrimSpace(c.PostForm("name"))
	if name != "" {
		a.SeriesModel.Update(id, name, slugifyStr(name))
	}
	c.Redirect(http.StatusFound, "/admin/series")
}

func (a *AdminHandler) AdminDeleteSeries(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	a.SeriesModel.Delete(id)
	c.Redirect(http.StatusFound, "/admin/series")
}

func (a *AdminHandler) AdminUpdateSeriesOrder(c *gin.Context) {
	seriesID, _ := strconv.Atoi(c.Param("id"))
	postID, _ := strconv.Atoi(c.PostForm("post_id"))
	sortOrder, _ := strconv.Atoi(c.PostForm("sort_order"))
	a.SeriesModel.UpdateSortOrder(postID, seriesID, sortOrder)
	c.Redirect(http.StatusFound, "/admin/series/"+c.Param("id")+"/posts")
}
