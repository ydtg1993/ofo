package admin

import (
	"ofo/models"
)

// AdminPageData holds data for admin template rendering.
type AdminPageData struct {
	Title             string
	Cfg               interface{}
	Error             string
	Success           string
	Posts             []models.Post
	Post              *models.Post
	Categories        []models.Category
	Tags              []models.Tag
	AllTags           []models.Tag
	Tag               *models.Tag
	TotalPosts        int
	IsEditing         bool
	IsNew             bool
	ShowCategories    bool
	ShowTags          bool
	ShowTagPosts      bool
	ShowSeries        bool
	ShowSeriesPosts   bool
	ShowResources     bool
	Resources         []models.Resource
	VideoCovers       map[int]string // resource ID → cover display URL (for video resources)
	SeriesList        []models.Series
	Series            *models.Series
	AllSeries         []models.Series
	PostSeries        *models.Series
	SeriesSortOrder   int
	PostCards         []models.PostCard
	Pagination        *models.Pagination
	IsQuick           bool
	PreselectedSeries *PreselectedSeriesInfo
}

// PreselectedSeriesInfo is passed to the editor form when navigating from series page.
type PreselectedSeriesInfo struct {
	ID        int
	Name      string
	NextOrder int
}
