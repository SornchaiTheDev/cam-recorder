# AGENTS.md - Developer Guidelines for cam-recorder

This file provides guidelines for AI agents working on this codebase.

## Project Overview

A Go-based IP camera recorder with RTSP support, web UI, and REST API. Built with Go 1.25+, Gin framework, and FFmpeg for video processing.

---

## Build, Lint, and Test Commands

### Makefile Commands

```bash
# Build the application
make build                    # Build for current platform
make build-all               # Build for Linux, Windows, macOS
make build-linux              # Build Linux amd64 and arm64
make build-windows            # Build Windows amd64 and arm64
make build-darwin             # Build macOS amd64 and arm64

# Development
make run                      # Run with config.yaml
make dev                      # Run in background
make deps                     # Install dependencies (tidy + download)

# Testing
make test                     # Run all tests (go test -v ./...)
make test -run TestName       # Run single test (via go test)

# Code quality
make fmt                      # Format code (go fmt ./...)
make lint                     # Run linter (go vet ./...)

# Cleanup
make clean                    # Remove binaries and recordings

# Docker
make docker-build            # Build Docker image
make docker-run              # Run container
```

### Manual Commands

```bash
# Single test file
go test -v ./internal/config/...

# Single test function
go test -v -run TestConfigLoad ./internal/config/...

# With coverage
go test -v -cover ./...

# Build with version
go build -ldflags "-s -w -X main.version=1.0.0" -o bin/cam-recorder ./cmd/main.go

# Run with custom config
go run ./cmd/main.go -config custom-config.yaml
```

---

## Code Style Guidelines

### Project Structure

```
cmd/                  # Entry points
internal/
  config/             # Configuration loading
  recorder/           # Camera recording logic
  storage/            # Storage management
  web/                # HTTP server and routes
web/
  static/             # CSS, JS
  templates/          # HTML templates
```

### Imports

- Group imports: stdlib first, then external packages
- Use explicit imports (no anonymous imports)
- Order alphabetically within groups

```go
import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/spf13/viper"

    "github.com/lets-vibe/cam-recorder/internal/config"
    "github.com/lets-vibe/cam-recorder/internal/recorder"
    "github.com/lets-vibe/cam-recorder/internal/storage"
)
```

### Naming Conventions

- **Files**: snake_case (e.g., `recorder.go`, `camera_config.go`)
- **Types/Interfaces**: PascalCase (e.g., `Recorder`, `CameraConfig`)
- **Functions/Methods**: PascalCase (e.g., `Start()`, `StopCamera()`)
- **Variables/Fields**: camelCase (e.g., `rtspURL`, `cameraName`)
- **Constants**: PascalCase or camelCase for unexported (e.g., `DefaultPort`)
- **Acronyms**: Keep original casing (e.g., `rtspURL`, not `rtspUrl`)

### Struct Tags

- Use `mapstructure` for config structs (matches Viper)
- Use `json` for API response structs
- Keep tags aligned when possible

```go
type CameraConfig struct {
    Name    string `mapstructure:"name"`
    RTSPURL string `mapstructure:"rtsp_url"`
    Enabled bool   `mapstructure:"enabled"`
}

type RecorderStatus struct {
    Running   bool   `json:"running"`
    Uptime    string `json:"uptime"`
    LastError string `json:"last_error,omitempty"`
}
```

### Error Handling

- Return errors with context using `%w` for wrapping
- Use sentinel errors for known conditions
- Handle errors at appropriate levels (don't ignore unless intentional)

```go
// Good - wrapped error
if err := os.MkdirAll(path, 0755); err != nil {
    return fmt.Errorf("failed to create output directory: %w", err)
}

// Good - check and return early
if rec.running {
    return fmt.Errorf("recorder already running")
}

// Use log for non-fatal errors
if err := store.Start(ctx); err != nil {
    log.Printf("Warning: Failed to start storage manager: %v", err)
}
```

### Context Usage

- Pass `context.Context` as first parameter to long-running operations
- Use `context.WithCancel()` for shutdown signals
- Check `ctx.Done()` in select statements for cancellation

```go
func (r *Recorder) Start(ctx context.Context) error {
    go r.runRecorder(ctx)
    return nil
}

func (r *Recorder) runRecorder(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            r.stopFFmpeg()
            return
        // ...
        }
    }
}
```

### Thread Safety

- Use `sync.Mutex` for mutable shared state
- Use `sync.RWMutex` when reads outnumber writes
- Always defer Unlock() after Lock()

```go
type Recorder struct {
    mu   sync.Mutex
    // ...
}

func (r *Recorder) Stop() {
    r.mu.Lock()
    defer r.mu.Unlock()
    // ...
}
```

### HTTP Routes (Gin)

- Use `gin.SetMode(gin.ReleaseMode)` in production
- Use Recovery() middleware
- Return appropriate status codes

```go
s.Router = gin.New()
s.Router.Use(gin.Recovery())

s.Router.GET("/api/status", s.handleStatus)
s.Router.POST("/api/camera/:name/start", s.handleCameraStart)
```

### Configuration

- Use Viper for config loading
- Set sensible defaults
- Validate required fields

```go
func Load(configPath string) (*Config, error) {
    v := viper.New()
    v.SetConfigFile(configPath)
    v.SetDefault("server.port", 8080)
    // ...
}
```

### Testing

- Test files should be named `*_test.go`
- Use table-driven tests when applicable
- Test both success and error paths

```go
func TestLoadConfig(t *testing.T) {
    tests := []struct {
        name    string
        path    string
        wantErr bool
    }{
        {"valid config", "config.yaml", false},
        {"missing config", "nonexistent.yaml", true},
    }
    // ...
}
```

### Logging

- Use `log` package for main application output
- Printf for user-facing messages
- Include context in error messages

```go
fmt.Printf("Starting web server on http://%s:%d\n", host, port)
log.Fatalf("Failed to load config: %v", err)
```

---

## External Dependencies

- **gin** (v1.11.0): HTTP web framework
- **viper** (v1.21.0): Configuration management
- **FFmpeg**: Video recording (external binary required)

---

## Common Tasks

### Adding a New API Endpoint

1. Add route in `internal/web/server.go`:
   ```go
   s.Router.GET("/api/newendpoint", s.handleNewEndpoint)
   ```

2. Add handler method in same file

3. Test with curl or browser

### Adding a New Camera Field

1. Add field to `CameraConfig` in `internal/config/config.go`
2. Add mapstructure tag
3. Add default in `Load()` if needed
4. Update example in `config.example.yaml`

### Adding a New Package

1. Create `internal/newpackage/`
2. Add `package newpackage`
3. Import where needed
4. Update Makefile if adding new test paths
