package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lets-vibe/cam-recorder/internal/config"
)

type Manager struct {
	config      *config.RecordingConfig
	stopCh      chan struct{}
	mu          sync.Mutex
	totalSize   int64
	lastCleanup time.Time
}

type StorageStats struct {
	TotalSize     int64                `json:"total_size_bytes"`
	TotalSizeHR   string               `json:"total_size_human"`
	FileCount     int                  `json:"file_count"`
	OldestFile    time.Time            `json:"oldest_file,omitempty"`
	NewestFile    time.Time            `json:"newest_file,omitempty"`
	LastCleanup   time.Time            `json:"last_cleanup"`
	RetentionDays int                  `json:"retention_days"`
	Cameras       []CameraStorageStats `json:"cameras"`
}

type CameraStorageStats struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SizeHR    string    `json:"size_human"`
	FileCount int       `json:"file_count"`
	Oldest    time.Time `json:"oldest,omitempty"`
	Newest    time.Time `json:"newest,omitempty"`
}

func NewManager(cfg *config.RecordingConfig) *Manager {
	return &Manager{
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if err := os.MkdirAll(m.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	go m.cleanupLoop(ctx)

	return nil
}

func (m *Manager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	m.cleanup()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastCleanup = time.Now()
	cutoff := time.Now().AddDate(0, 0, -m.config.RetentionDays)

	cameraDirs, err := os.ReadDir(m.config.OutputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var deletedCount int
	var deletedSize int64

	for _, cameraDir := range cameraDirs {
		if !cameraDir.IsDir() {
			continue
		}

		cameraPath := filepath.Join(m.config.OutputDir, cameraDir.Name())
		entries, err := os.ReadDir(cameraPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(cutoff) {
				filePath := filepath.Join(cameraPath, entry.Name())
				if err := os.Remove(filePath); err != nil {
					fmt.Printf("failed to delete %s: %v\n", filePath, err)
					continue
				}
				deletedCount++
				deletedSize += info.Size()
			}
		}
	}

	if deletedCount > 0 {
		fmt.Printf("Cleanup: deleted %d files (%s)\n", deletedCount, formatBytes(deletedSize))
	}

	return nil
}

func (m *Manager) Stop() {
	close(m.stopCh)
}

func (m *Manager) GetStats() (*StorageStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := &StorageStats{
		RetentionDays: m.config.RetentionDays,
		LastCleanup:   m.lastCleanup,
		Cameras:       []CameraStorageStats{},
	}

	cameraDirs, err := os.ReadDir(m.config.OutputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return nil, err
	}

	var totalSize int64
	var totalFileCount int
	var oldestTime, newestTime time.Time

	for _, cameraDir := range cameraDirs {
		if !cameraDir.IsDir() {
			continue
		}

		cameraName := cameraDir.Name()
		cameraPath := filepath.Join(m.config.OutputDir, cameraName)

		cameraStats := m.getCameraStats(cameraName, cameraPath)
		stats.Cameras = append(stats.Cameras, cameraStats)

		totalSize += cameraStats.Size
		totalFileCount += cameraStats.FileCount

		if !cameraStats.Oldest.IsZero() {
			if oldestTime.IsZero() || cameraStats.Oldest.Before(oldestTime) {
				oldestTime = cameraStats.Oldest
			}
		}
		if !cameraStats.Newest.IsZero() {
			if newestTime.IsZero() || cameraStats.Newest.After(newestTime) {
				newestTime = cameraStats.Newest
			}
		}
	}

	stats.TotalSize = totalSize
	stats.TotalSizeHR = formatBytes(totalSize)
	stats.FileCount = totalFileCount
	stats.OldestFile = oldestTime
	stats.NewestFile = newestTime

	m.totalSize = totalSize

	return stats, nil
}

func (m *Manager) getCameraStats(name, path string) CameraStorageStats {
	stats := CameraStorageStats{
		Name: name,
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return stats
	}

	var totalSize int64
	var oldestTime, newestTime time.Time
	fileCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), "."+m.config.Format) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		totalSize += info.Size()
		fileCount++

		modTime := info.ModTime()
		if oldestTime.IsZero() || modTime.Before(oldestTime) {
			oldestTime = modTime
		}
		if newestTime.IsZero() || modTime.After(newestTime) {
			newestTime = modTime
		}
	}

	stats.Size = totalSize
	stats.SizeHR = formatBytes(totalSize)
	stats.FileCount = fileCount
	stats.Oldest = oldestTime
	stats.Newest = newestTime

	return stats
}

func (m *Manager) ListFiles(cameraName, filter string, limit int) ([]FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var searchDir string
	if cameraName != "" {
		searchDir = filepath.Join(m.config.OutputDir, strings.ReplaceAll(cameraName, " ", "_"))
	} else {
		searchDir = m.config.OutputDir
	}

	var files []FileInfo

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(info.Name(), "."+m.config.Format) {
			return nil
		}

		name := info.Name()
		if filter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
			return nil
		}

		relPath, _ := filepath.Rel(m.config.OutputDir, path)
		cameraFromPath := ""
		if parts := strings.Split(relPath, string(os.PathSeparator)); len(parts) > 1 {
			cameraFromPath = strings.ReplaceAll(parts[0], "_", " ")
		}

		files = append(files, FileInfo{
			Name:       name,
			CameraName: cameraFromPath,
			Path:       path,
			Size:       info.Size(),
			SizeHR:     formatBytes(info.Size()),
			CreatedAt:  info.ModTime(),
		})

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sortFilesByDateDesc(files)

	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	return files, nil
}

func (m *Manager) DeleteFile(cameraName, filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filePath string
	if cameraName != "" {
		filePath = filepath.Join(m.config.OutputDir, strings.ReplaceAll(cameraName, " ", "_"), filename)
	} else {
		filePath = filepath.Join(m.config.OutputDir, filename)
	}

	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(m.config.OutputDir)) {
		return fmt.Errorf("invalid file path")
	}

	return os.Remove(filePath)
}

func (m *Manager) GetFilePath(cameraName, filename string) (string, error) {
	var filePath string
	if cameraName != "" {
		filePath = filepath.Join(m.config.OutputDir, strings.ReplaceAll(cameraName, " ", "_"), filename)
	} else {
		filePath = filepath.Join(m.config.OutputDir, filename)
	}

	if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(m.config.OutputDir)) {
		return "", fmt.Errorf("invalid file path")
	}

	if _, err := os.Stat(filePath); err != nil {
		return "", err
	}

	return filePath, nil
}

func (m *Manager) GetCameraDir(cameraName string) string {
	return filepath.Join(m.config.OutputDir, strings.ReplaceAll(cameraName, " ", "_"))
}

type FileInfo struct {
	Name       string    `json:"name"`
	CameraName string    `json:"camera_name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	SizeHR     string    `json:"size_human"`
	CreatedAt  time.Time `json:"created_at"`
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func sortFilesByDateDesc(files []FileInfo) {
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].CreatedAt.Before(files[j].CreatedAt) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}
