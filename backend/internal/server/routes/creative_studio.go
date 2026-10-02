package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/creativestudio"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// RegisterCreativeStudioRoutes serves owner-scoped projects and durable creative
// jobs. Call it after gateway routes have been registered: the executor reuses
// those routes, including normal authentication, scheduling and billing.
func RegisterCreativeStudioRoutes(r *gin.Engine, jwtAuth middleware.JWTAuthMiddleware, apiKeys *service.APIKeyService, settings *service.SettingService, cfg *config.Config, limiter *middleware.PanelRateLimiter) {
	executor := &creativeStudioExecutor{gateway: r, apiKeys: apiKeys, renders: make(chan struct{}, 1)}
	var store *creativestudio.Store
	var initErr error
	if cfg.Pricing.DataDir != "" {
		store, initErr = creativestudio.New(filepath.Join(cfg.Pricing.DataDir, "creative-studio"), executor, creativestudio.Options{MaxWorkers: 2, PerUserWorkers: 2})
	} else {
		initErr = errors.New("data directory is not configured")
	}
	if initErr != nil {
		slog.Error("creative studio storage unavailable", "error", initErr)
	}
	api := r.Group("/api/v1/studio/projects")
	api.Use(gin.HandlerFunc(jwtAuth), middleware.BackendModeUserGuard(settings), limiter.Global())
	api.Use(func(c *gin.Context) {
		if store == nil {
			response.Error(c, http.StatusServiceUnavailable, "Creative studio storage is unavailable")
			c.Abort()
			return
		}
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			response.Unauthorized(c, "User not authenticated")
			c.Abort()
			return
		}
		c.Next()
	})
	owner := func(c *gin.Context) int64 {
		subject, _ := middleware.GetAuthSubjectFromContext(c)
		return subject.UserID
	}
	reply := func(c *gin.Context, value any, err error) {
		if err != nil {
			creativeStudioError(c, err)
			return
		}
		response.Success(c, value)
	}
	api.GET("/capabilities", func(c *gin.Context) {
		kind := c.Query("kind")
		if kind != "image" && kind != "video" {
			response.BadRequest(c, "Invalid media kind")
			return
		}
		response.Success(c, creativestudio.ModelCapabilities(kind, c.Query("model"), c.Query("platform")))
	})
	api.GET("", func(c *gin.Context) { v, err := store.List(owner(c)); reply(c, v, err) })
	api.POST("", middleware.RequestBodyLimit(1<<20), func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid project")
			return
		}
		v, err := store.Create(owner(c), req.Name)
		reply(c, v, err)
	})
	api.GET("/:project", func(c *gin.Context) { v, err := store.Get(owner(c), c.Param("project")); reply(c, v, err) })
	api.PUT("/:project", middleware.RequestBodyLimit(2<<20), func(c *gin.Context) {
		var req creativestudio.Project
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid project")
			return
		}
		v, err := store.Save(owner(c), c.Param("project"), req)
		reply(c, v, err)
	})
	api.DELETE("/:project", func(c *gin.Context) { reply(c, gin.H{}, store.Delete(owner(c), c.Param("project"))) })
	api.POST("/:project/assets", middleware.RequestBodyLimit((128<<20)+(1<<20)), func(c *gin.Context) {
		header, err := c.FormFile("file")
		if c.Request.MultipartForm != nil {
			defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
		}
		if err != nil {
			response.BadRequest(c, "A media file is required")
			return
		}
		f, err := header.Open()
		if err != nil {
			response.BadRequest(c, "Cannot read media file")
			return
		}
		defer func() { _ = f.Close() }()
		data, err := io.ReadAll(io.LimitReader(f, (128<<20)+1))
		if err != nil || len(data) == 0 {
			response.BadRequest(c, "Cannot read media file")
			return
		}
		if len(data) > 128<<20 {
			response.Error(c, http.StatusRequestEntityTooLarge, "Media exceeds 128 MB")
			return
		}
		contentType := http.DetectContentType(data)
		if contentType == "audio/wave" {
			contentType = "audio/wav"
		}
		if contentType == "application/ogg" {
			contentType = "audio/ogg"
		}
		if !creativeMediaTypeAllowed(contentType) {
			response.BadRequest(c, "Unsupported media type")
			return
		}
		if strings.HasPrefix(contentType, "image/") && len(data) > 32<<20 {
			response.Error(c, http.StatusRequestEntityTooLarge, "Image exceeds 32 MB")
			return
		}
		v, err := store.PutAsset(owner(c), c.Param("project"), filepath.Base(header.Filename), contentType, data)
		reply(c, v, err)
	})
	api.GET("/:project/assets/:asset", func(c *gin.Context) {
		asset, err := store.OpenAsset(owner(c), c.Param("project"), c.Param("asset"))
		if err != nil {
			creativeStudioError(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		if c.Query("thumbnail") == "1" && asset.Kind == "image" {
			data, err := creativeThumbnail(asset.Path)
			if err != nil {
				response.BadRequest(c, "Image preview is unavailable")
				return
			}
			c.Data(http.StatusOK, "image/jpeg", data)
			return
		}
		f, err := os.Open(asset.Path)
		if err != nil {
			response.NotFound(c, "Asset not found")
			return
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil {
			response.NotFound(c, "Asset not found")
			return
		}
		c.Header("Content-Type", asset.MIME)
		c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": asset.Name}))
		http.ServeContent(c.Writer, c.Request, asset.Name, info.ModTime(), f)
	})
	api.POST("/:project/preflight", middleware.RequestBodyLimit(1<<20), func(c *gin.Context) {
		var req struct {
			NodeIDs  []string `json:"node_ids"`
			Revision int64    `json:"revision"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid workflow request")
			return
		}
		v, err := store.Preflight(owner(c), c.Param("project"), req.NodeIDs, req.Revision)
		if err != nil {
			reply(c, v, err)
			return
		}
		groups, err := loadCreativeStudioGroups(c.Request.Context(), apiKeys, owner(c))
		if err != nil {
			reply(c, v, err)
			return
		}
		if issues := groups.issues(v.Nodes); len(issues) > 0 {
			v.Valid = false
			v.Errors = append(v.Errors, issues...)
		}
		reply(c, v, nil)
	})
	api.POST("/:project/runs", middleware.RequestBodyLimit(1<<20), func(c *gin.Context) {
		var req struct {
			RequestID string   `json:"request_id"`
			NodeIDs   []string `json:"node_ids"`
			Revision  int64    `json:"revision"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid workflow request")
			return
		}
		groups, err := loadCreativeStudioGroups(c.Request.Context(), apiKeys, owner(c))
		if err != nil {
			creativeStudioError(c, err)
			return
		}
		v, err := store.RunAtRevisionValidated(owner(c), c.Param("project"), req.RequestID, req.NodeIDs, req.Revision, groups.validate)
		reply(c, v, err)
	})
	api.POST("/:project/jobs/:job/retry", func(c *gin.Context) {
		v, err := store.Retry(owner(c), c.Param("project"), c.Param("job"))
		reply(c, v, err)
	})
	api.POST("/:project/jobs/:job/cancel", func(c *gin.Context) {
		v, err := store.Cancel(owner(c), c.Param("project"), c.Param("job"))
		reply(c, v, err)
	})
	api.POST("/:project/exports", middleware.RequestBodyLimit(1<<20), func(c *gin.Context) {
		var req creativeComposition
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid timeline")
			return
		}
		ids, err := req.validate()
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		assets := make(map[string]creativestudio.AssetFile, len(ids))
		for _, id := range ids {
			asset, err := store.OpenAsset(owner(c), c.Param("project"), id)
			if err != nil {
				creativeStudioError(c, err)
				return
			}
			assets[id] = asset
		}
		if err := req.validateAssets(assets); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		raw, err := json.Marshal(req)
		if err != nil {
			response.BadRequest(c, "Invalid timeline")
			return
		}
		v, err := store.Enqueue(owner(c), c.Param("project"), req.RequestID, "export", "compose", "", raw, ids)
		reply(c, v, err)
	})
	if store != nil {
		creativeStudioWorkers.add(store)
		store.Start()
	}
}

func creativeStudioError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, creativestudio.ErrNotFound):
		response.NotFound(c, "Project, task or asset not found")
	case errors.Is(err, creativestudio.ErrConflict):
		response.Error(c, http.StatusConflict, "Project has newer changes. Reload before saving.")
	case errors.Is(err, creativestudio.ErrInvalid):
		response.BadRequest(c, err.Error())
	case errors.Is(err, creativestudio.ErrQuota):
		response.Error(c, http.StatusRequestEntityTooLarge, "Project or media storage limit reached")
	case errors.Is(err, creativestudio.ErrBusy):
		response.Error(c, http.StatusConflict, "Tasks are still running or the task queue is full")
	case errors.Is(err, creativestudio.ErrClosed):
		response.Error(c, http.StatusServiceUnavailable, "Creative studio is shutting down")
	default:
		slog.Error("creative studio request failed", "error", err)
		response.InternalError(c, "Unable to save or load the creative project")
	}
}

func creativeMediaTypeAllowed(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/webp", "video/mp4", "video/webm", "audio/mpeg", "audio/wave", "audio/wav", "audio/ogg", "application/ogg":
		return true
	default:
		return false
	}
}

func creativeThumbnail(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil, errors.New("image is too large to preview")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	width, height := cfg.Width, cfg.Height
	if width > 640 || height > 640 {
		if width >= height {
			height = height * 640 / width
			width = 640
		} else {
			width = width * 640 / height
			height = 640
		}
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 80})
	return out.Bytes(), err
}
