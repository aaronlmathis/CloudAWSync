/*
SPDX-License-Identifier: GPL-3.0-or-later

Copyright (C) 2025 Aaron Mathis aaron@deepthought.sh

This file is part of CloudAWSync.

CloudAWSync is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

CloudAWSync is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with CloudAWSync. If not, see https://www.gnu.org/licenses/.
*/

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"CloudAWSync/internal/interfaces"

	"gopkg.in/yaml.v3"
)

// AWSConfig holds AWS-specific configuration
type AWSConfig struct {
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	SessionToken    string `yaml:"session_token"`
	S3Bucket        string `yaml:"s3_bucket"`
	S3Prefix        string `yaml:"s3_prefix"`
	Endpoint        string `yaml:"endpoint"` // for S3-compatible services
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, text
	OutputPath string `yaml:"output_path"` // file path or stdout
	MaxSize    int    `yaml:"max_size"`    // MB
	MaxAge     int    `yaml:"max_age"`     // days
	MaxBackups int    `yaml:"max_backups"`
	Compress   bool   `yaml:"compress"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Port            int           `yaml:"port"`
	Path            string        `yaml:"path"`
	CollectInterval time.Duration `yaml:"collect_interval"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	EncryptionEnabled bool     `yaml:"encryption_enabled"`
	EncryptionKey     string   `yaml:"encryption_key"`
	MaxFileSize       int64    `yaml:"max_file_size"` // bytes
	AllowedExtensions []string `yaml:"allowed_extensions"`
	DeniedExtensions  []string `yaml:"denied_extensions"`
}

// PerformanceConfig holds performance tuning configuration
type PerformanceConfig struct {
	MaxConcurrentUploads   int           `yaml:"max_concurrent_uploads"`
	MaxConcurrentDownloads int           `yaml:"max_concurrent_downloads"`
	UploadChunkSize        int64         `yaml:"upload_chunk_size"`
	DownloadChunkSize      int64         `yaml:"download_chunk_size"`
	RetryAttempts          int           `yaml:"retry_attempts"`
	RetryDelay             time.Duration `yaml:"retry_delay"`
	TimeoutDuration        time.Duration `yaml:"timeout_duration"`
	BandwidthLimit         int64         `yaml:"bandwidth_limit"` // bytes per second
}

// Config represents the main configuration structure
type Config struct {
	AWS         AWSConfig                  `yaml:"aws"`
	Logging     LoggingConfig              `yaml:"logging"`
	Metrics     MetricsConfig              `yaml:"metrics"`
	Security    SecurityConfig             `yaml:"security"`
	Performance PerformanceConfig          `yaml:"performance"`
	Directories []interfaces.SyncDirectory `yaml:"directories"`
	SystemD     SystemDConfig              `yaml:"systemd"`
}

// SystemDConfig holds systemd-specific configuration
type SystemDConfig struct {
	ServiceName   string `yaml:"service_name"`
	WorkingDir    string `yaml:"working_dir"`
	User          string `yaml:"user"`
	Group         string `yaml:"group"`
	RestartPolicy string `yaml:"restart_policy"`
	LogLevel      string `yaml:"log_level"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		AWS: AWSConfig{
			Region:   "us-east-1",
			S3Prefix: "cloudawsync/",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			OutputPath: "/var/log/cloudawsync/cloudawsync.log",
			MaxSize:    100,
			MaxAge:     30,
			MaxBackups: 10,
			Compress:   true,
		},
		Metrics: MetricsConfig{
			Enabled:         true,
			Port:            9090,
			Path:            "/metrics",
			CollectInterval: 30 * time.Second,
		},
		Security: SecurityConfig{
			EncryptionEnabled: true,
			MaxFileSize:       100 * 1024 * 1024, // 100MB
			AllowedExtensions: []string{},
			DeniedExtensions:  []string{".tmp", ".lock"},
		},
		Performance: PerformanceConfig{
			MaxConcurrentUploads:   5,
			MaxConcurrentDownloads: 5,
			UploadChunkSize:        5 * 1024 * 1024, // 5MB
			DownloadChunkSize:      5 * 1024 * 1024, // 5MB
			RetryAttempts:          3,
			RetryDelay:             5 * time.Second,
			TimeoutDuration:        30 * time.Second,
			BandwidthLimit:         0, // unlimited
		},
		SystemD: SystemDConfig{
			ServiceName:   "cloudawsync",
			WorkingDir:    "/opt/cloudawsync",
			User:          "cloudawsync",
			Group:         "cloudawsync",
			RestartPolicy: "always",
			LogLevel:      "info",
		},
	}
}

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return config, nil // return default config if file doesn't exist
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to file
func (c *Config) SaveConfig(configPath string) error {
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.AWS.S3Bucket == "" {
		return fmt.Errorf("AWS S3 bucket is required")
	}

	if c.Performance.MaxConcurrentUploads <= 0 {
		return fmt.Errorf("max concurrent uploads must be positive")
	}

	if c.Performance.MaxConcurrentDownloads <= 0 {
		return fmt.Errorf("max concurrent downloads must be positive")
	}

	if c.Performance.UploadChunkSize <= 0 {
		return fmt.Errorf("upload chunk size must be positive")
	}

	if c.Performance.DownloadChunkSize <= 0 {
		return fmt.Errorf("download chunk size must be positive")
	}

	for i, dir := range c.Directories {
		if dir.LocalPath == "" {
			return fmt.Errorf("directory %d: local path is required", i)
		}

		if !filepath.IsAbs(dir.LocalPath) {
			return fmt.Errorf("directory %d: local path must be absolute", i)
		}

		if dir.SyncMode != interfaces.SyncModeRealtime &&
			dir.SyncMode != interfaces.SyncModeScheduled &&
			dir.SyncMode != interfaces.SyncModeBoth {
			return fmt.Errorf("directory %d: invalid sync mode", i)
		}

		if dir.SyncMode == interfaces.SyncModeScheduled || dir.SyncMode == interfaces.SyncModeBoth {
			if dir.Schedule == "" {
				return fmt.Errorf("directory %d: schedule is required for scheduled sync mode", i)
			}
		}
	}

	return nil
}

// NewConfig creates a new configuration instance
func NewConfig(ctx context.Context) *Config {
	return DefaultConfig()
}

func getDefaultConfigPath() string {
	if configDir := os.Getenv("XDG_CONFIG_HOME"); configDir != "" {
		return filepath.Join(configDir, "cloudawsync", "config.yaml")
	}

	if homeDir := os.Getenv("HOME"); homeDir != "" {
		return filepath.Join(homeDir, ".config", "cloudawsync", "config.yaml")
	}

	return "/etc/cloudawsync/config.yaml"
}
