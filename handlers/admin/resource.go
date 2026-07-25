package admin

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ofo/handlers"
	"ofo/logger"
	"ofo/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- Resource Management ----

func (a *AdminHandler) AdminResources(c *gin.Context) {
	total, _ := a.ResourceModel.CountAll()
	pg := adminPagination(c, total, 20)
	resources, _ := a.ResourceModel.ListAll((pg.CurrentPage-1)*pg.PerPage, pg.PerPage)
	c.HTML(http.StatusOK, "admin_resources.html", AdminPageData{
		Title: "资源管理", Cfg: a.Cfg, Resources: resources, Pagination: pg, ShowResources: true,
	})
}

func (a *AdminHandler) AdminDeleteResource(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	resources, _ := a.ResourceModel.ListAll(0, 10000)
	for _, r := range resources {
		if r.ID == id {
			if r.Storage == "" || r.Storage == a.Cfg.StorageBackend {
				a.Storage.Delete(c.Request.Context(), strings.TrimPrefix(r.URL, "/"))
			}
			a.ResourceModel.Delete(id)
			break
		}
	}
	c.Redirect(http.StatusFound, "/admin/resources")
}

// ---- File Upload ----

func (a *AdminHandler) AdminUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".mp4": true, ".webm": true, ".ogg": true, ".mov": true,
	}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件类型：" + ext})
		return
	}

	// Generate unique filename (basename only, stored in DB without date folder)
	dbFilename := uuid.New().String() + ext

	// Upload via storage backend with year/month prefix
	datePrefix := time.Now().Format("2006/01")
	key := "uploads/" + datePrefix + "/" + dbFilename
	url, err := a.Storage.Upload(c.Request.Context(), key, file, header.Size)
	if err != nil {
		logger.ErrorWithContext(c, "failed to upload file", "name", dbFilename, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
		return
	}

	// 记录到资源表
	mimeType := models.MIMEType(ext)
	if _, err := a.ResourceModel.Create(dbFilename, url, a.Cfg.StorageBackend, header.Size, mimeType); err != nil {
		logger.ErrorWithContext(c, "failed to record uploaded resource in database", "name", dbFilename, "err", err)
	}

	c.JSON(http.StatusOK, gin.H{"url": a.Storage.PublicURL(url)})
}

// AdminCleanupUploads deletes uploads that have been removed from the editor
// (i.e., uploaded during an editing session but the user clicked "Cancel").
// Accepts JSON: {"urls": ["url1", "url2", ...]}
// Only deletes resources that are NOT linked to any post (safety check).
func (a *AdminHandler) AdminCleanupUploads(c *gin.Context) {
	var body struct {
		URLs []string `json:"urls"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.URLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid urls"})
		return
	}

	deleted := 0
	for _, url := range body.URLs {
		if url == "" {
			continue
		}
		relPath := handlers.URLToRelativePath(url, a.Storage, a.Cfg)
		r, err := a.ResourceModel.FindByURL(relPath)
		if err != nil {
			continue // not found or already cleaned up
		}

		// Safety: only delete if no post references this resource
		noLinks, _ := a.ResourceModel.HasNoLinks(r.ID)
		if !noLinks {
			continue // still linked to a post
		}

		// Delete from storage (derive full path from URL, which includes date subdirectories)
		storageKey := strings.TrimPrefix(r.URL, "/")
		if r.Storage != "" && r.Storage != a.Cfg.StorageBackend {
			logger.Info("skip cleanup: resource on different backend",
				"filename", r.Filename, "resourceStorage", r.Storage, "currentBackend", a.Cfg.StorageBackend)
			deleted++
			continue
		}
		if err := a.Storage.Delete(c.Request.Context(), storageKey); err != nil {
			logger.ErrorWithContext(c, "failed to delete upload file from storage", "key", storageKey, "err", err)
		} else {
			a.ResourceModel.Delete(r.ID)
			deleted++
			logger.Info("cleaned up orphan upload", "filename", r.Filename)
		}
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
