package admin

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ofo/handlers"
	"ofo/logger"
	"ofo/media"
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
			a.cleanupResourceTree(c.Request.Context(), &r)
			break
		}
	}
	c.Redirect(http.StatusFound, "/admin/resources")
}

// cleanupResourceTree deletes a resource from storage and DB.  When the
// resource is an HLS playlist (.m3u8) it also removes every associated .ts
// segment via the video_segments table.
func (a *AdminHandler) cleanupResourceTree(ctx context.Context, r *models.Resource) {

	// 1. Delete the main resource file from storage.
	if r.Storage == "" || r.Storage == a.Cfg.StorageBackend {
		a.Storage.Delete(ctx, strings.TrimPrefix(r.URL, "/"))
	}

	// 2. For m3u8 playlists, clean up related TS segments from the dedicated table.
	if strings.HasSuffix(strings.ToLower(r.URL), ".m3u8") {
		vs, err := a.VideoSegmentModel.FindByResourceID(r.ID)
		if err == nil && vs != nil {
			segments, _ := vs.SegmentsList()
			for _, segKey := range segments {
				a.Storage.Delete(ctx, segKey)
			}
			a.VideoSegmentModel.DeleteByResourceID(r.ID)
		}
	}

	// 3. Remove the DB record.
	a.ResourceModel.Delete(r.ID)
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

	// Generate unique filename (basename only)
	dbFilename := uuid.New().String() + ext
	datePrefix := time.Now().Format("2006/01")
	key := "uploads/" + datePrefix + "/" + dbFilename

	// Save uploaded file to a temporary location so we can probe / segment it.
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, dbFilename)
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		logger.ErrorWithContext(c, "failed to create temp file", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存临时文件失败"})
		return
	}
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		logger.ErrorWithContext(c, "failed to write temp file", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入临时文件失败"})
		return
	}
	tmpFile.Close()
	defer os.Remove(tmpPath) // clean up when we are done

	// ---- Video segmentation (HLS) for files > 30 s ----
	var (
		finalURL    string
		resourceURL string
		mediaType   = "image"
		isHLS       bool
	)

	if media.IsVideoExt(ext) {
		mediaType = "video"
		duration, durErr := media.GetVideoDuration(tmpPath)
		if durErr == nil && duration > media.HLSThreshold {
			baseName := strings.TrimSuffix(dbFilename, ext)

			// HLS files go into a dedicated subdirectory:
			//   uploads/2026/07/{uuid}/{uuid}.m3u8
			//   uploads/2026/07/{uuid}/{uuid}_000.ts
			hlsDir := "uploads/" + datePrefix + "/" + baseName

			outDir := filepath.Join(tmpDir, "hls_"+baseName)
			m3u8Path, tsFiles, segErr := media.SegmentVideo(tmpPath, outDir, baseName)
			if segErr != nil {
				logger.ErrorWithContext(c, "video segmentation failed, falling back to direct upload", "err", segErr)
				os.RemoveAll(outDir)
			} else {
				defer os.RemoveAll(outDir)

				// Upload m3u8
				m3u8Name := baseName + ".m3u8"
				m3u8Key := hlsDir + "/" + m3u8Name
				m3u8F, _ := os.Open(m3u8Path)
				if m3u8F != nil {
					a.Storage.Upload(c.Request.Context(), m3u8Key, m3u8F, 0)
					m3u8F.Close()
				}
				resourceURL = "/" + m3u8Key

				// Upload TS segments + collect their storage keys
				var tsKeys []string
				var tsTotalSize int64
				for _, ts := range tsFiles {
					tsName := filepath.Base(ts)
					tsKey := hlsDir + "/" + tsName
					tsF, _ := os.Open(ts)
					if tsF != nil {
						fi, _ := tsF.Stat()
						if fi != nil {
							tsTotalSize += fi.Size()
						}
						a.Storage.Upload(c.Request.Context(), tsKey, tsF, 0)
						tsF.Close()
					}
					tsKeys = append(tsKeys, tsKey)
				}

				// Create resource record for the m3u8 playlist.
				m3u8Fi, _ := os.Stat(m3u8Path)
				m3u8Size := int64(0)
				if m3u8Fi != nil {
					m3u8Size = m3u8Fi.Size()
				}
				resID, _ := a.ResourceModel.Create(m3u8Name, resourceURL, a.Cfg.StorageBackend, m3u8Size+tsTotalSize, models.MIMEType(".m3u8"))

				// Store TS paths in the dedicated video_segments table (JSON array).
				a.VideoSegmentModel.Create(int(resID), tsKeys)

				finalURL = a.Storage.PublicURL(resourceURL)
				isHLS = true
				mediaType = "video_hls"
			}
		}
	}

	// ---- Direct upload (short video / image / segmentation failure) ----
	if !isHLS {
		uploadF, openErr := os.Open(tmpPath)
		if openErr != nil {
			logger.ErrorWithContext(c, "failed to re-open temp file for upload", "err", openErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
			return
		}
		defer uploadF.Close()

		var uploadErr error
		finalURL, uploadErr = a.Storage.Upload(c.Request.Context(), key, uploadF, header.Size)
		if uploadErr != nil {
			logger.ErrorWithContext(c, "failed to upload file", "name", dbFilename, "err", uploadErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
			return
		}
		resourceURL = finalURL

		mimeType := models.MIMEType(ext)
		if _, err := a.ResourceModel.Create(dbFilename, finalURL, a.Cfg.StorageBackend, header.Size, mimeType); err != nil {
			logger.ErrorWithContext(c, "failed to record uploaded resource in database", "name", dbFilename, "err", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"url":  finalURL,
		"rel":  resourceURL,
		"type": mediaType,
	})
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

		if r.Storage != "" && r.Storage != a.Cfg.StorageBackend {
			logger.Info("skip cleanup: resource on different backend",
				"filename", r.Filename, "resourceStorage", r.Storage, "currentBackend", a.Cfg.StorageBackend)
			deleted++
			continue
		}

		a.cleanupResourceTree(c.Request.Context(), r)
		deleted++
		logger.Info("cleaned up orphan upload", "filename", r.Filename)
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
