package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lets-vibe/cam-recorder/internal/config"
	"github.com/lets-vibe/cam-recorder/internal/recorder"
	"github.com/lets-vibe/cam-recorder/internal/storage"
)

type Server struct {
	config   *config.Config
	recorder *recorder.RecorderManager
	storage  *storage.Manager
	mjpeg    *recorder.MJPEGManager
	Router   *gin.Engine
}

func NewServer(cfg *config.Config, rec *recorder.RecorderManager, store *storage.Manager) *Server {
	s := &Server{
		config:   cfg,
		recorder: rec,
		storage:  store,
		mjpeg:    recorder.NewMJPEGManager(),
	}

	gin.SetMode(gin.ReleaseMode)
	s.Router = gin.New()
	s.Router.Use(gin.Recovery())

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	s.Router.Static("/static", "./web/static")
	s.Router.LoadHTMLGlob("./web/templates/*")

	s.Router.GET("/", s.handleIndex)
	s.Router.GET("/camera/:name", s.handleCameraDetail)
	s.Router.GET("/live/:name", s.handleLiveStream)
	s.Router.GET("/recordings", s.handleRecordingsAPI)
	s.Router.GET("/recordings/list", s.handleRecordingsPage)
	s.Router.GET("/dl/:camera/:filename", s.handleDownload)
	s.Router.GET("/play/:camera/:filename", s.handlePlay)
	s.Router.DELETE("/recordings/:camera/:filename", s.handleDelete)
	s.Router.GET("/api/status", s.handleStatus)
	s.Router.GET("/api/status/:name", s.handleCameraStatus)
	s.Router.GET("/api/storage", s.handleStorageStats)
	s.Router.POST("/api/camera/:name/start", s.handleCameraStart)
	s.Router.POST("/api/camera/:name/stop", s.handleCameraStop)
}

func (s *Server) Start(ctx context.Context) error {
	for _, cam := range s.config.Cameras {
		if cam.Enabled {
			go s.mjpeg.Start(ctx, cam.Name, cam.RTSPURL)
		}
	}

	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	return s.Router.Run(addr)
}

func (s *Server) handleIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"pageTitle": "Camera Recorder",
		"cameras":   s.config.Cameras,
	})
}

func (s *Server) handleCameraDetail(c *gin.Context) {
	cameraName := c.Param("name")

	var camera *config.CameraConfig
	for i := range s.config.Cameras {
		if s.config.Cameras[i].Name == cameraName {
			camera = &s.config.Cameras[i]
			break
		}
	}

	if camera == nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Camera not found"})
		return
	}

	rec, _ := s.recorder.GetRecorder(cameraName)

	c.HTML(http.StatusOK, "camera.html", gin.H{
		"pageTitle": cameraName + " - Camera Recorder",
		"camera":    camera,
		"recorder":  rec,
	})
}

func (s *Server) handleLiveStream(c *gin.Context) {
	cameraName := c.Param("name")

	c.Header("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.String(http.StatusInternalServerError, "Streaming not supported")
		return
	}

	cond, ok := s.mjpeg.GetCond(cameraName)
	if !ok {
		c.String(http.StatusNotFound, "Camera not found")
		return
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		default:
			cond.L.Lock()
			cond.Wait()
			frame, ok := s.mjpeg.GetFrame(cameraName)
			cond.L.Unlock()

			if !ok || len(frame) == 0 {
				continue
			}

			_, err := fmt.Fprintf(c.Writer, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
			if err != nil {
				return
			}

			if _, err := c.Writer.Write(frame); err != nil {
				return
			}

			_, err = fmt.Fprint(c.Writer, "\r\n")
			if err != nil {
				return
			}

			flusher.Flush()
		}
	}
}

func (s *Server) handleRecordingsAPI(c *gin.Context) {
	cameraName := c.Query("camera")
	filter := c.Query("filter")
	limitStr := c.DefaultQuery("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	files, err := s.storage.ListFiles(cameraName, filter, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"recordings": files,
		"count":      len(files),
	})
}

func (s *Server) handleRecordingsPage(c *gin.Context) {
	cameraName := c.Query("camera")
	filter := c.Query("filter")
	limitStr := c.DefaultQuery("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	files, err := s.storage.ListFiles(cameraName, filter, limit)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": err.Error()})
		return
	}

	c.HTML(http.StatusOK, "recordings.html", gin.H{
		"pageTitle":   "Recordings",
		"cameras":     s.config.Cameras,
		"recordings":  files,
		"selectedCam": cameraName,
	})
}

func (s *Server) handleDownload(c *gin.Context) {
	cameraName := c.Param("camera")
	filename := c.Param("filename")

	filePath, err := s.storage.GetFilePath(cameraName, filename)
	if err != nil {
		c.String(http.StatusNotFound, "File not found")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(filePath)
}

func (s *Server) handlePlay(c *gin.Context) {
	cameraName := c.Param("camera")
	filename := c.Param("filename")

	_, err := s.storage.GetFilePath(cameraName, filename)
	if err != nil {
		c.String(http.StatusNotFound, "File not found")
		return
	}

	c.HTML(http.StatusOK, "player.html", gin.H{
		"pageTitle":  "Play Recording",
		"cameraName": cameraName,
		"filename":   filename,
		"videoUrl":   fmt.Sprintf("/dl/%s/%s", cameraName, filename),
	})
}

func (s *Server) handleDelete(c *gin.Context) {
	cameraName := c.Param("camera")
	filename := c.Param("filename")

	if err := s.storage.DeleteFile(cameraName, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File deleted", "filename": filename})
}

func (s *Server) handleStatus(c *gin.Context) {
	status := gin.H{
		"cameras":     []gin.H{},
		"server_time": time.Now().Format(time.RFC3339),
	}

	recorderStatus := s.recorder.GetStatus()
	cameras := []gin.H{}

	for _, cam := range s.config.Cameras {
		recStatus, exists := recorderStatus[cam.Name]
		camStatus := gin.H{
			"name":      cam.Name,
			"enabled":   cam.Enabled,
			"connected": false,
			"streaming": s.mjpeg.IsRunning(cam.Name),
		}

		if exists {
			camStatus["connected"] = recStatus.Running
			camStatus["running"] = recStatus.Running
			camStatus["uptime"] = recStatus.Uptime
			if recStatus.LastError != "" {
				camStatus["last_error"] = recStatus.LastError
			}
		}

		cameras = append(cameras, camStatus)
	}

	status["cameras"] = cameras

	c.JSON(http.StatusOK, status)
}

func (s *Server) handleCameraStatus(c *gin.Context) {
	cameraName := c.Param("name")

	rec, exists := s.recorder.GetRecorder(cameraName)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Camera not found"})
		return
	}

	var lastErr string
	if err := rec.GetLastError(); err != nil {
		lastErr = err.Error()
	}

	c.JSON(http.StatusOK, gin.H{
		"name":       cameraName,
		"running":    rec.IsRunning(),
		"uptime":     rec.Uptime().String(),
		"last_error": lastErr,
		"streaming":  s.mjpeg.IsRunning(cameraName),
	})
}

func (s *Server) handleStorageStats(c *gin.Context) {
	stats, err := s.storage.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleCameraStart(c *gin.Context) {
	cameraName := c.Param("name")

	if err := s.recorder.StartCamera(c.Request.Context(), cameraName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var rtspURL string
	for _, cam := range s.config.Cameras {
		if cam.Name == cameraName {
			rtspURL = cam.RTSPURL
			break
		}
	}

	if rtspURL != "" {
		go s.mjpeg.Start(c.Request.Context(), cameraName, rtspURL)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Camera started", "camera": cameraName})
}

func (s *Server) handleCameraStop(c *gin.Context) {
	cameraName := c.Param("name")

	s.recorder.StopCamera(cameraName)
	s.mjpeg.Stop(cameraName)

	c.JSON(http.StatusOK, gin.H{"message": "Camera stopped", "camera": cameraName})
}

type TemplateData struct {
	PageTitle  string
	CameraName string
	Error      string
}
