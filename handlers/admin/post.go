package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ofo/handlers"
	"ofo/logger"

	"github.com/gin-gonic/gin"
)

// ---- Dashboard ----

func (a *AdminHandler) AdminDashboard(c *gin.Context) {
	total, err := a.PostModel.CountAll()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count posts", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage

	posts, err := a.PostModel.ListAllPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list paginated posts", "err", err)
		c.HTML(http.StatusInternalServerError, "admin_dashboard.html", AdminPageData{
			Title: "Dashboard",
			Cfg:   a.Cfg,
			Error: "加载文章列表失败",
		})
		return
	}

	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for dashboard", "err", err)
	}

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        a.Cfg,
		Posts:      posts,
		Categories: categories,
		Pagination: pg,
	})
}

// ---- New Post Form ----

func (a *AdminHandler) AdminNewPost(c *gin.Context) {
	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for new post", "err", err)
	}
	allTags, err := a.PostModel.AllTagsSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for new post", "err", err)
	}
	allSeries, _ := a.SeriesModel.All()

	// 从系列管理页跳转时预填系列
	var preselected *PreselectedSeriesInfo
	if sid, _ := strconv.Atoi(c.Query("series_id")); sid > 0 {
		for _, s := range allSeries {
			if s.ID == sid {
				preselected = &PreselectedSeriesInfo{ID: s.ID, Name: s.Name, NextOrder: s.PostCount + 1}
				break
			}
		}
	}

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:             "New Post",
		Cfg:               a.Cfg,
		IsNew:             true,
		Categories:        categories,
		AllTags:           allTags,
		AllSeries:         allSeries,
		PreselectedSeries: preselected,
	})
}

// ---- Quick Publish (速览模式) ----

func (a *AdminHandler) AdminQuickPublish(c *gin.Context) {
	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for quick publish", "err", err)
	}
	allSeries, _ := a.SeriesModel.All()

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:      "快速发布",
		Cfg:        a.Cfg,
		IsNew:      true,
		IsQuick:    true,
		Categories: categories,
		AllSeries:  allSeries,
	})
}

func (a *AdminHandler) AdminQuickCreatePost(c *gin.Context) {
	caption := strings.TrimSpace(c.PostForm("caption"))
	imageURL := strings.TrimSpace(c.PostForm("image_url"))
	categoryIDStr := c.PostForm("category_id")

	if caption == "" {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "快速发布",
			Cfg:        a.Cfg,
			IsNew:      true,
			IsQuick:    true,
			Categories: categories,
			Error:      "请输入内容描述",
		})
		return
	}

	// 标题：caption 前 30 字
	title := caption
	runes := []rune(title)
	if len(runes) > 30 {
		title = string(runes[:30])
	}

	slug := slugifyStr(title)

	// 内容：图片 + 文字
	contentMD := caption
	if imageURL != "" {
		contentMD = "![" + title + "](" + imageURL + ")\n\n" + caption
	}
	contentHTML := renderMarkdown(contentMD)
	contentHTML = handlers.NormalizeContentURLs(contentHTML, a.Storage, a.Cfg)
	excerpt := extractExcerptStr(caption, 200)

	thumbnailURL := imageURL

	var categoryID *int
	if categoryIDStr != "" {
		if cid, err := strconv.Atoi(categoryIDStr); err == nil {
			categoryID = &cid
		}
	}

	publishAt := parseDateTime(c.PostForm("publish_at"))

	contentHTML = handlers.NormalizeContentURLs(contentHTML, a.Storage, a.Cfg)
	contentHTML = ResolveContentURLs(contentHTML, a.ResourceModel, a.Cfg)
	postID, err := a.PostModel.Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, true, publishAt, time.Now(), nil)
	if err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "快速发布",
			Cfg:        a.Cfg,
			IsNew:      true,
			IsQuick:    true,
			Categories: categories,
			Error:      "保存失败：" + err.Error(),
		})
		return
	}

	// 关联上传资源
	if err := a.ResourceModel.SyncPostResources(int(postID), contentHTML, a.Storage.IsStorageURL, func(url string) error {
		return a.Storage.Delete(c.Request.Context(), strings.TrimPrefix(url, "/"))
	}); err != nil {
		logger.ErrorWithContext(c, "failed to sync resources for quick post", "postID", postID, "err", err)
	}

	a.adminDashboardWithSuccess(c, "快速发布成功")
}

// ---- Edit Post Form ----

func (a *AdminHandler) AdminEditPost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	post, err := a.PostModel.GetByID(id)
	if err != nil {
		logger.WarnWithContext(c, "failed to get post for edit", "id", id, "err", err)
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for edit", "err", err)
	}
	tags, err := a.PostModel.TagsForPost(id)
	if err != nil {
		logger.ErrorWithContext(c, "failed to load tags for edit", "postID", id, "err", err)
	}
	allTags, err := a.PostModel.AllTagsSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load all tags for edit", "err", err)
	}
	allSeries, _ := a.SeriesModel.All()

	// 查找该文章所属系列及序号
	postSeries, sortOrder, _ := a.SeriesModel.GetPostSeries(id)

	c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
		Title:           "Edit: " + post.Title,
		Cfg:             a.Cfg,
		Post:            post,
		Categories:      categories,
		Tags:            tags,
		AllTags:         allTags,
		AllSeries:       allSeries,
		IsEditing:       true,
		PostSeries:      postSeries,
		SeriesSortOrder: sortOrder,
	})
}

// ---- Create Post ----

func (a *AdminHandler) AdminCreatePost(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagIDsStr := c.PostForm("tag_ids")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	// Render markdown
	contentHTML := renderMarkdown(contentMD)
	contentHTML = handlers.NormalizeContentURLs(contentHTML, a.Storage, a.Cfg)
	contentHTML = ResolveContentURLs(contentHTML, a.ResourceModel, a.Cfg)

	// Excerpt
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = title
	}

	var categoryID *int
	if categoryIDStr != "" {
		if cid, err := strconv.Atoi(categoryIDStr); err == nil {
			categoryID = &cid
		}
	}

	tagIDs, err := a.resolveTagIDs(tagIDsStr)
	if err != nil {
		tagIDs = nil
	}

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))
	publishAt := parseDateTime(c.PostForm("publish_at"))

	postID, err := a.PostModel.Create(title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, publishAt, createdAt, tagIDs)
	if err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:      "New Post",
			Cfg:        a.Cfg,
			IsNew:      true,
			Categories: categories,
			Error:      "保存失败：" + err.Error(),
		})
		return
	}

	// 关联上传资源到文章
	if err := a.ResourceModel.SyncPostResources(int(postID), contentHTML, a.Storage.IsStorageURL, func(url string) error {
		return a.Storage.Delete(c.Request.Context(), strings.TrimPrefix(url, "/"))
	}); err != nil {
		logger.ErrorWithContext(c, "failed to sync resources for new post", "postID", postID, "err", err)
	}

	// 关联系列
	if seriesID, _ := strconv.Atoi(c.PostForm("series_id")); seriesID > 0 {
		order, _ := strconv.Atoi(c.PostForm("series_order"))
		if order < 1 {
			order = 1
		}
		a.SeriesModel.LinkPost(int(postID), seriesID, order)
	}

	// Redirect to dashboard with success
	a.adminDashboardWithSuccess(c, "文章发布成功")
}

// ---- Update Post ----

func (a *AdminHandler) AdminUpdatePost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	slug := strings.TrimSpace(c.PostForm("slug"))
	contentMD := c.PostForm("content")
	categoryIDStr := c.PostForm("category_id")
	published := c.PostForm("published") == "1"
	tagIDsStr := c.PostForm("tag_ids")
	thumbnailURL := strings.TrimSpace(c.PostForm("thumbnail_url"))

	if slug == "" {
		slug = slugifyStr(title)
	}

	contentHTML := renderMarkdown(contentMD)
	contentHTML = handlers.NormalizeContentURLs(contentHTML, a.Storage, a.Cfg)
	contentHTML = ResolveContentURLs(contentHTML, a.ResourceModel, a.Cfg)
	excerpt := strings.TrimSpace(c.PostForm("excerpt"))
	if excerpt == "" {
		excerpt = title
	}

	var categoryID *int
	if categoryIDStr != "" {
		if cid, err := strconv.Atoi(categoryIDStr); err == nil {
			categoryID = &cid
		}
	}

	tagIDs, err := a.resolveTagIDs(tagIDsStr)
	if err != nil {
		tagIDs = nil
	}

	// 发布时间（默认当天）
	createdAt := parseDate(c.PostForm("created_at"))
	publishAt := parseDateTime(c.PostForm("publish_at"))

	if err := a.PostModel.Update(id, title, slug, contentMD, contentHTML, excerpt, thumbnailURL, categoryID, published, publishAt, createdAt, tagIDs); err != nil {
		categories, _ := a.PostModel.AllCategoriesSimple()
		tags, _ := a.PostModel.TagsForPost(id)
		post, _ := a.PostModel.GetByID(id)
		allTags, _ := a.PostModel.AllTagsSimple()
		allSeries, _ := a.SeriesModel.All()
		postSeries, sortOrder, _ := a.SeriesModel.GetPostSeries(id)
		c.HTML(http.StatusOK, "admin_editor.html", AdminPageData{
			Title:           "Edit: " + title,
			Cfg:             a.Cfg,
			Post:            post,
			Categories:      categories,
			Tags:            tags,
			AllTags:         allTags,
			AllSeries:       allSeries,
			IsEditing:       true,
			PostSeries:      postSeries,
			SeriesSortOrder: sortOrder,
			Error:           "更新失败：" + err.Error(),
		})
		return
	}

	// 同步上传资源关联
	if err := a.ResourceModel.SyncPostResources(id, contentHTML, a.Storage.IsStorageURL, func(url string) error {
		return a.Storage.Delete(c.Request.Context(), strings.TrimPrefix(url, "/"))
	}); err != nil {
		logger.ErrorWithContext(c, "failed to sync resources for updated post", "postID", id, "err", err)
	}

	// 关联系列
	if seriesID, _ := strconv.Atoi(c.PostForm("series_id")); seriesID > 0 {
		order, _ := strconv.Atoi(c.PostForm("series_order"))
		if order < 1 {
			order = 1
		}
		a.SeriesModel.UpdateSortOrder(id, seriesID, order)
	}

	a.adminDashboardWithSuccess(c, "文章更新成功")
}

// ---- Delete Post ----

func (a *AdminHandler) AdminDeletePost(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// 1. 查找文章关联的资源（用于后续清理）
	resources, err := a.ResourceModel.FindResourcesByPostID(id)
	if err != nil {
		logger.ErrorWithContext(c, "failed to find resources for post deletion", "postID", id, "err", err)
	}

	// 2. 删除文章（CASCADE 会自动清理 post_tags、post_resources、post_series）
	if err := a.PostModel.Delete(id); err != nil {
		a.adminDashboardWithSuccess(c, "删除文章失败")
		return
	}

	// 3. 清理已无任何关联的资源（文件 + 记录）
	if len(resources) > 0 {
		currentBackend := a.Cfg.StorageBackend
		go func() {
			for _, r := range resources {
				// 检查是否还有其他文章引用此资源
				noLinks, _ := a.ResourceModel.HasNoLinks(r.ID)
				if !noLinks {
					continue // 仍被其他文章引用，保留
				}
				if r.Storage != "" && r.Storage != currentBackend {
					logger.Info("skip deleting resource on different backend",
						"filename", r.Filename, "resourceStorage", r.Storage, "currentBackend", currentBackend)
					continue
				}
				key := strings.TrimPrefix(r.URL, "/")
				if err := a.Storage.Delete(context.Background(), key); err != nil {
					logger.Error("failed to delete resource file", "filename", r.Filename, "err", err)
				} else {
					// 文件删除成功，删除数据库记录
					a.ResourceModel.Delete(r.ID)
				}
			}
		}()
	}

	a.adminDashboardWithSuccess(c, "文章删除成功")
}

// ---- Private Helpers ----

func (a *AdminHandler) adminDashboardWithSuccess(c *gin.Context, msg string) {
	total, err := a.PostModel.CountAll()
	if err != nil {
		logger.ErrorWithContext(c, "failed to count posts for dashboard", "err", err)
		total = 0
	}
	pg := adminPagination(c, total, 15)
	offset := (pg.CurrentPage - 1) * pg.PerPage
	posts, err := a.PostModel.ListAllPaginated(offset, pg.PerPage)
	if err != nil {
		logger.ErrorWithContext(c, "failed to list posts for dashboard", "err", err)
	}
	categories, err := a.PostModel.AllCategoriesSimple()
	if err != nil {
		logger.ErrorWithContext(c, "failed to load categories for dashboard", "err", err)
	}

	c.HTML(http.StatusOK, "admin_dashboard.html", AdminPageData{
		Title:      "Dashboard",
		Cfg:        a.Cfg,
		Posts:      posts,
		Categories: categories,
		Success:    msg,
		Pagination: pg,
	})
}
