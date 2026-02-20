package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Cameras   []CameraConfig  `mapstructure:"cameras"`
	Recording RecordingConfig `mapstructure:"recording"`
	Server    ServerConfig    `mapstructure:"server"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

type CameraConfig struct {
	Name    string `mapstructure:"name"`
	RTSPURL string `mapstructure:"rtsp_url"`
	Enabled bool   `mapstructure:"enabled"`
}

type RecordingConfig struct {
	SegmentDuration time.Duration `mapstructure:"segment_duration"`
	RetentionDays   int           `mapstructure:"retention_days"`
	OutputDir       string        `mapstructure:"output_dir"`
	Format          string        `mapstructure:"format"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	v.SetDefault("cameras", []CameraConfig{})
	v.SetDefault("recording.segment_duration", "5m")
	v.SetDefault("recording.retention_days", 7)
	v.SetDefault("recording.output_dir", "./recordings")
	v.SetDefault("recording.format", "mp4")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("logging.level", "info")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	for i := range cfg.Cameras {
		if cfg.Cameras[i].Name == "" {
			cfg.Cameras[i].Name = "Camera"
		}
	}

	return &cfg, nil
}
