package admin

import (
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// cleanupResourceTree deletes a resource from storage and DB.  For videos
// (which live in their own subdirectory) the entire directory is removed in
// one shot — video file, HLS segments, and cover.jpg.
func (a *AdminHandler) cleanupResourceTree(ctx context.Context, r *models.Resource) {
	url := r.URL
	ext := strings.ToLower(filepath.Ext(url))
	isVideo := media.IsVideoExt(ext) || ext == ".m3u8"

	if r.Storage == "" || r.Storage == a.Cfg.StorageBackend {
		if isVideo {
			// Delete the entire {uuid}/ directory.
			dir := filepath.Dir(url)
			a.Storage.DeletePrefix(ctx, strings.TrimPrefix(dir, "/"))
		} else {
			// Image or other single file.
			a.Storage.Delete(ctx, strings.TrimPrefix(url, "/"))
		}
	}

	// Clean up video_segments row if present.
	if isVideo {
		a.VideoSegmentModel.DeleteByResourceID(r.ID)
	}

	// Remove the DB record.
	a.ResourceModel.Delete(r.ID)
}

// ---- File Upload ----

// sendProgress writes a JSON progress event followed by a newline, then
// flushes so the client receives it immediately (chunked transfer).
func sendProgress(c *gin.Context, phase string, msg string, current, total int) {
	data, _ := json.Marshal(map[string]interface{}{
		"phase":   phase,
		"msg":     msg,
		"current": current,
		"total":   total,
	})
	c.Writer.Write(data)
	c.Writer.Write([]byte("\n"))
	c.Writer.Flush()
}

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
		hlsDir      string
		resID       int64
		baseName    string
	)

	if media.IsVideoExt(ext) {
		mediaType = "video"
		baseName = strings.TrimSuffix(dbFilename, ext)
		// Videos always use a subdirectory: uploads/2026/07/{uuid}/{uuid}.mp4
		key = "uploads/" + datePrefix + "/" + baseName + "/" + dbFilename
		duration, durErr := media.GetVideoDuration(tmpPath)
		if durErr == nil && duration > media.HLSThreshold {

			sendProgress(c, "probing", "检测到长视频（"+strconv.Itoa(int(duration))+"秒），准备切片...", 0, 0)

			// HLS files go into a dedicated subdirectory:
			//   uploads/2026/07/{uuid}/{uuid}.m3u8
			//   uploads/2026/07/{uuid}/{uuid}_000.ts
			hlsDir = "uploads/" + datePrefix + "/" + baseName

			sendProgress(c, "segmenting", "正在切片...", 0, 0)
			outDir := filepath.Join(tmpDir, "hls_"+baseName)
			m3u8Path, tsFiles, segErr := media.SegmentVideo(tmpPath, outDir, baseName)
			if segErr != nil {
				logger.ErrorWithContext(c, "video segmentation failed, falling back to direct upload", "err", segErr)
				os.RemoveAll(outDir)
			} else {
				defer os.RemoveAll(outDir)

				sendProgress(c, "uploading", "正在上传 m3u8...", 0, len(tsFiles)+1)

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
				for i, ts := range tsFiles {
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
					sendProgress(c, "uploading", "上传切片 "+(strconv.Itoa(i+1))+"/"+strconv.Itoa(len(tsFiles))+"...", i+1, len(tsFiles)+1)
				}

				// Create resource record for the m3u8 playlist.
				m3u8Fi, _ := os.Stat(m3u8Path)
				m3u8Size := int64(0)
				if m3u8Fi != nil {
					m3u8Size = m3u8Fi.Size()
				}
				resID, _ = a.ResourceModel.Create(m3u8Name, resourceURL, a.Cfg.StorageBackend, m3u8Size+tsTotalSize, models.MIMEType(".m3u8"))

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

		sendProgress(c, "uploading", "正在上传...", 0, 0)
		var uploadErr error
		finalURL, uploadErr = a.Storage.Upload(c.Request.Context(), key, uploadF, header.Size)
		if uploadErr != nil {
			logger.ErrorWithContext(c, "failed to upload file", "name", dbFilename, "err", uploadErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败"})
			return
		}
		// resourceURL is the clean relative path (/uploads/...), not the display URL.
		resourceURL = "/" + key

		mimeType := models.MIMEType(ext)
		resID, _ = a.ResourceModel.Create(dbFilename, resourceURL, a.Cfg.StorageBackend, header.Size, mimeType)
	}

	// ---- Generate video poster + store cover dimensions ----
	if media.IsVideoExt(ext) && resID > 0 {
		posterTmpPath := filepath.Join(tmpDir, "cover.jpg")
		if err := media.GenerateVideoPoster(tmpPath, posterTmpPath); err != nil {
			logger.Warn("failed to generate video poster, skipping", "video", dbFilename, "err", err)
		} else {
			defer os.Remove(posterTmpPath)

			var posterKey string
			if isHLS {
				posterKey = hlsDir + "/cover.jpg"
			} else {
				posterKey = "uploads/" + datePrefix + "/" + baseName + "/cover.jpg"
			}

			posterF, openErr := os.Open(posterTmpPath)
			if openErr == nil {
				if _, upErr := a.Storage.Upload(c.Request.Context(), posterKey, posterF, 0); upErr != nil {
					logger.Warn("failed to upload video poster", "key", posterKey, "err", upErr)
				}
				posterF.Close()
			}

			// Read cover dimensions from the generated JPEG and store in DB.
			w, h := getImageDimensions(posterTmpPath)
			coverInfo := &models.CoverInfo{
				URL:    "/" + posterKey,
				Width:  w,
				Height: h,
			}
			if isHLS {
				// Update existing video_segments row with cover info.
				if err := a.VideoSegmentModel.UpsertCover(int(resID), coverInfo); err != nil {
					logger.Warn("failed to save cover info for HLS", "err", err)
				}
			} else {
				// Create video_segments row (empty segments) with cover.
				if err := a.VideoSegmentModel.UpsertCover(int(resID), coverInfo); err != nil {
					logger.Warn("failed to save cover info for short video", "err", err)
				}
			}
		}
	}

	// Send final result as the last progress event.
	result, _ := json.Marshal(map[string]interface{}{
		"phase": "done",
		"url":   finalURL,
		"rel":   resourceURL,
		"type":  mediaType,
	})
	c.Writer.Write(result)
	c.Writer.Write([]byte("\n"))
	c.Writer.Flush()
}

// getImageDimensions reads the pixel width and height of a local image file.
func getImageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
