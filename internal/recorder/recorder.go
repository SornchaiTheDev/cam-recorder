package recorder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lets-vibe/cam-recorder/internal/config"
)

type Recorder struct {
	config     *config.RecordingConfig
	rtspURL    string
	cameraName string
	outputDir  string
	cmd        *exec.Cmd
	stopCh     chan struct{}
	mu         sync.Mutex
	running    bool
	lastError  error
	startTime  time.Time
}

type RecordingSegment struct {
	Filename   string    `json:"filename"`
	CameraName string    `json:"camera_name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	Duration   string    `json:"duration"`
}

func New(rtspURL, cameraName string, cfg *config.RecordingConfig) *Recorder {
	safeName := strings.ReplaceAll(cameraName, " ", "_")
	outputDir := filepath.Join(cfg.OutputDir, safeName)

	return &Recorder{
		config:     cfg,
		rtspURL:    rtspURL,
		cameraName: cameraName,
		outputDir:  outputDir,
		stopCh:     make(chan struct{}),
	}
}

func (r *Recorder) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("recorder already running")
	}

	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	go r.runRecorder(ctx)
	r.running = true
	r.startTime = time.Now()

	return nil
}

func (r *Recorder) runRecorder(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.stopFFmpeg()
			return
		case <-r.stopCh:
			r.stopFFmpeg()
			return
		default:
			if err := r.recordSegment(ctx); err != nil {
				r.mu.Lock()
				r.lastError = err
				r.mu.Unlock()
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (r *Recorder) recordSegment(ctx context.Context) error {
	timestamp := time.Now().Format("20060102_150405")
	safeName := strings.ReplaceAll(r.cameraName, " ", "_")
	filename := fmt.Sprintf("%s_%s.%s",
		safeName,
		timestamp,
		r.config.Format,
	)
	outputPath := filepath.Join(r.outputDir, filename)

	segmentDuration := int(r.config.SegmentDuration.Seconds())

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", r.rtspURL,
		// transcode video to H.264 and audio to AAC for broad container compatibility
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-t", fmt.Sprintf("%d", segmentDuration),
		"-movflags", "+faststart",
		"-y",
		outputPath,
	}

	r.cmd = exec.CommandContext(ctx, "ffmpeg", args...)

	if err := r.cmd.Run(); err != nil {
		if ctx.Err() == context.Canceled {
			return nil
		}
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	return nil
}

func (r *Recorder) stopFFmpeg() {
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Signal(os.Interrupt)
		r.cmd.Wait()
	}
}

func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	close(r.stopCh)
	r.running = false
}

func (r *Recorder) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Recorder) GetLastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastError
}

func (r *Recorder) Uptime() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return 0
	}
	return time.Since(r.startTime)
}

func (r *Recorder) CameraName() string {
	return r.cameraName
}

func (r *Recorder) OutputDir() string {
	return r.outputDir
}

func (r *Recorder) ListSegments() ([]RecordingSegment, error) {
	var segments []RecordingSegment

	entries, err := os.ReadDir(r.outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return segments, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, "."+r.config.Format) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		segments = append(segments, RecordingSegment{
			Filename:   name,
			CameraName: r.cameraName,
			Path:       filepath.Join(r.outputDir, name),
			Size:       info.Size(),
			CreatedAt:  info.ModTime(),
			Duration:   r.config.SegmentDuration.String(),
		})
	}

	return segments, nil
}

type RecorderManager struct {
	config    *config.RecordingConfig
	recorders map[string]*Recorder
	mu        sync.RWMutex
}

func NewRecorderManager(cfg *config.RecordingConfig) *RecorderManager {
	return &RecorderManager{
		config:    cfg,
		recorders: make(map[string]*Recorder),
	}
}

func (rm *RecorderManager) AddCamera(ctx context.Context, name, rtspURL string, enabled bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.recorders[name]; exists {
		return fmt.Errorf("camera %s already exists", name)
	}

	rec := New(rtspURL, name, rm.config)
	rm.recorders[name] = rec

	if enabled {
		if err := rec.Start(ctx); err != nil {
			return fmt.Errorf("failed to start recorder for %s: %w", name, err)
		}
	}

	return nil
}

func (rm *RecorderManager) RemoveCamera(name string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rec, exists := rm.recorders[name]; exists {
		rec.Stop()
		delete(rm.recorders, name)
	}
}

func (rm *RecorderManager) StartCamera(ctx context.Context, name string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rec, exists := rm.recorders[name]
	if !exists {
		return fmt.Errorf("camera %s not found", name)
	}

	return rec.Start(ctx)
}

func (rm *RecorderManager) StopCamera(name string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rec, exists := rm.recorders[name]; exists {
		rec.Stop()
	}
}

func (rm *RecorderManager) GetRecorder(name string) (*Recorder, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	rec, ok := rm.recorders[name]
	return rec, ok
}

func (rm *RecorderManager) GetAllRecorders() map[string]*Recorder {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make(map[string]*Recorder)
	for k, v := range rm.recorders {
		result[k] = v
	}
	return result
}

func (rm *RecorderManager) StopAll() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, rec := range rm.recorders {
		rec.Stop()
	}
}

func (rm *RecorderManager) ListAllSegments() ([]RecordingSegment, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var allSegments []RecordingSegment

	for _, rec := range rm.recorders {
		segments, err := rec.ListSegments()
		if err != nil {
			continue
		}
		allSegments = append(allSegments, segments...)
	}

	sortSegmentsByDateDesc(allSegments)

	return allSegments, nil
}

func (rm *RecorderManager) GetStatus() map[string]RecorderStatus {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	status := make(map[string]RecorderStatus)
	for name, rec := range rm.recorders {
		var lastErr string
		if err := rec.GetLastError(); err != nil {
			lastErr = err.Error()
		}
		status[name] = RecorderStatus{
			Running:   rec.IsRunning(),
			Uptime:    rec.Uptime().String(),
			LastError: lastErr,
			OutputDir: rec.OutputDir(),
		}
	}
	return status
}

type RecorderStatus struct {
	Running   bool   `json:"running"`
	Uptime    string `json:"uptime"`
	LastError string `json:"last_error,omitempty"`
	OutputDir string `json:"output_dir"`
}

func sortSegmentsByDateDesc(segments []RecordingSegment) {
	for i := 0; i < len(segments)-1; i++ {
		for j := i + 1; j < len(segments); j++ {
			if segments[i].CreatedAt.Before(segments[j].CreatedAt) {
				segments[i], segments[j] = segments[j], segments[i]
			}
		}
	}
}

type MJPEGStreamer struct {
	rtspURL string
	cmd     *exec.Cmd
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

func NewMJPEGStreamer(rtspURL string) *MJPEGStreamer {
	return &MJPEGStreamer{
		rtspURL: rtspURL,
		stopCh:  make(chan struct{}),
	}
}

func (m *MJPEGStreamer) Start(ctx context.Context, frameCallback func([]byte)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("streamer already running")
	}

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", m.rtspURL,
		"-vf", "fps=10,scale=640:-1",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"-f", "image2pipe",
		"-",
	}

	m.cmd = exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	m.running = true

	go m.readFrames(stdout, frameCallback)

	return nil
}

func (m *MJPEGStreamer) readFrames(stdout interface{ Read([]byte) (int, error) }, callback func([]byte)) {
	buffer := make([]byte, 1024*1024)
	accumulator := make([]byte, 0, 1024*1024)

	for {
		select {
		case <-m.stopCh:
			return
		default:
			n, err := stdout.Read(buffer)
			if err != nil {
				return
			}

			accumulator = append(accumulator, buffer[:n]...)

			for {
				startIdx := bytesIndex(accumulator, []byte{0xFF, 0xD8})
				if startIdx == -1 {
					if len(accumulator) > 512*1024 {
						accumulator = accumulator[len(accumulator)/2:]
					}
					break
				}

				endIdx := bytesIndex(accumulator[startIdx:], []byte{0xFF, 0xD9})
				if endIdx == -1 {
					break
				}

				endIdx += startIdx + 2
				frame := make([]byte, endIdx-startIdx)
				copy(frame, accumulator[startIdx:endIdx])

				if callback != nil {
					callback(frame)
				}

				accumulator = accumulator[endIdx:]
			}
		}
	}
}

func (m *MJPEGStreamer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopCh)
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Signal(os.Interrupt)
		m.cmd.Wait()
	}
	m.running = false
}

func (m *MJPEGStreamer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func bytesIndex(data, pattern []byte) int {
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

type MJPEGManager struct {
	streamers map[string]*MJPEGStreamer
	frames    map[string][]byte
	conds     map[string]*sync.Cond
	mu        sync.RWMutex
}

func NewMJPEGManager() *MJPEGManager {
	return &MJPEGManager{
		streamers: make(map[string]*MJPEGStreamer),
		frames:    make(map[string][]byte),
		conds:     make(map[string]*sync.Cond),
	}
}

func (m *MJPEGManager) Start(ctx context.Context, name, rtspURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.streamers[name]; exists {
		return nil
	}

	streamer := NewMJPEGStreamer(rtspURL)
	m.streamers[name] = streamer
	m.frames[name] = nil
	m.conds[name] = sync.NewCond(&sync.Mutex{})

	go streamer.Start(ctx, func(frame []byte) {
		m.mu.RLock()
		cond, ok := m.conds[name]
		m.mu.RUnlock()

		if !ok {
			return
		}

		cond.L.Lock()
		m.mu.Lock()
		m.frames[name] = make([]byte, len(frame))
		copy(m.frames[name], frame)
		m.mu.Unlock()
		cond.Broadcast()
		cond.L.Unlock()
	})

	return nil
}

func (m *MJPEGManager) Stop(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if streamer, exists := m.streamers[name]; exists {
		streamer.Stop()
		delete(m.streamers, name)
		delete(m.frames, name)
		delete(m.conds, name)
	}
}

func (m *MJPEGManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, streamer := range m.streamers {
		streamer.Stop()
	}
	m.streamers = make(map[string]*MJPEGStreamer)
	m.frames = make(map[string][]byte)
	m.conds = make(map[string]*sync.Cond)
}

func (m *MJPEGManager) GetFrame(name string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	frame, ok := m.frames[name]
	return frame, ok
}

func (m *MJPEGManager) GetCond(name string) (*sync.Cond, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cond, ok := m.conds[name]
	return cond, ok
}

func (m *MJPEGManager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if streamer, ok := m.streamers[name]; ok {
		return streamer.IsRunning()
	}
	return false
}
