package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lets-vibe/cam-recorder/internal/config"
	"github.com/lets-vibe/cam-recorder/internal/recorder"
	"github.com/lets-vibe/cam-recorder/internal/storage"
	"github.com/lets-vibe/cam-recorder/internal/web"
)

var (
	configPath = flag.String("config", "config.yaml", "Path to configuration file")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	fmt.Printf("IP Camera Recorder v%s\n", version)
	fmt.Println("=====================================")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Cameras configured: %d\n", len(cfg.Cameras))
	for _, cam := range cfg.Cameras {
		status := "disabled"
		if cam.Enabled {
			status = "enabled"
		}
		fmt.Printf("  - %s (%s)\n", cam.Name, status)
	}
	fmt.Printf("Output directory: %s\n", cfg.Recording.OutputDir)
	fmt.Printf("Segment duration: %v\n", cfg.Recording.SegmentDuration)
	fmt.Printf("Retention: %d days\n", cfg.Recording.RetentionDays)
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := storage.NewManager(&cfg.Recording)
	if err := store.Start(ctx); err != nil {
		log.Fatalf("Failed to start storage manager: %v", err)
	}
	fmt.Println("✓ Storage manager started")

	recManager := recorder.NewRecorderManager(&cfg.Recording)

	for _, cam := range cfg.Cameras {
		if err := recManager.AddCamera(ctx, cam.Name, cam.RTSPURL, cam.Enabled); err != nil {
			log.Printf("Warning: Failed to add camera %s: %v", cam.Name, err)
		} else {
			status := "added"
			if cam.Enabled {
				status = "started"
			}
			fmt.Printf("✓ Camera '%s' %s\n", cam.Name, status)
		}
	}

	server := web.NewServer(cfg, recManager, store)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n=====================================")
		fmt.Println("Shutting down...")
		cancel()
		recManager.StopAll()
		store.Stop()
	}()

	fmt.Println()
	fmt.Printf("Starting web server on http://%s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	if err := server.Start(ctx); err != nil {
		log.Printf("Server stopped: %v", err)
	}

	fmt.Println("Goodbye!")
}
