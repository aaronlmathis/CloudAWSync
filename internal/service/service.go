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

package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CloudAWSync/internal/config"
	"CloudAWSync/internal/engine"
	"CloudAWSync/internal/interfaces"
	"CloudAWSync/internal/metrics"
	"CloudAWSync/internal/providers"
	"CloudAWSync/internal/utils"
	"CloudAWSync/internal/watcher"

	"go.uber.org/zap"
)

// Service represents the main CloudAWSync service
type Service struct {
	config *config.Config
	logger *zap.Logger

	// Components
	provider interfaces.CloudProvider
	watcher  interfaces.FileWatcher
	metrics  interfaces.MetricsCollector
	engine   interfaces.SyncEngine

	// State
	running bool
	mutex   sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewService creates a new CloudAWSync service
func NewService(cfg *config.Config) (*Service, error) {
	// Initialize logger
	logger, err := utils.InitLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	service := &Service{
		config: cfg,
		logger: logger,
	}

	// Initialize components
	if err := service.initializeComponents(); err != nil {
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	logger.Info("CloudAWSync service created successfully")
	return service, nil
}

// Start starts the CloudAWSync service
func (s *Service) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("service is already running")
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.logger.Info("Starting CloudAWSync service")

	// Start metrics collector
	if s.config.Metrics.Enabled && s.metrics != nil {
		if err := s.metrics.Start(s.ctx); err != nil {
			s.logger.Error("Failed to start metrics collector", zap.Error(err))
			return fmt.Errorf("failed to start metrics collector: %w", err)
		}
		s.logger.Info("Metrics collector started",
			zap.Int("port", s.config.Metrics.Port),
			zap.String("path", s.config.Metrics.Path))
	}

	// Add directories to sync engine
	for _, dir := range s.config.Directories {
		if engineImpl, ok := s.engine.(*engine.Engine); ok {
			engineImpl.AddDirectory(dir)
			s.logger.Info("Added directory to sync engine",
				zap.String("local_path", dir.LocalPath),
				zap.String("remote_path", dir.RemotePath),
				zap.String("sync_mode", string(dir.SyncMode)))
		}
	}

	// Start sync engine
	if err := s.engine.Start(s.ctx); err != nil {
		s.logger.Error("Failed to start sync engine", zap.Error(err))
		return fmt.Errorf("failed to start sync engine: %w", err)
	}
	s.logger.Info("Sync engine started successfully")

	// Perform initial sync for all directories in the background
	go s.performInitialSync()

	s.running = true
	s.logger.Info("CloudAWSync service started successfully")

	return nil
}

// Stop stops the CloudAWSync service
func (s *Service) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping CloudAWSync service")

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Stop sync engine
	if s.engine != nil {
		if err := s.engine.Stop(); err != nil {
			s.logger.Error("Failed to stop sync engine", zap.Error(err))
		}
	}

	// Stop metrics collector
	if s.config.Metrics.Enabled && s.metrics != nil {
		if collector, ok := s.metrics.(*metrics.PrometheusCollector); ok {
			if err := collector.Stop(); err != nil {
				s.logger.Error("Failed to stop metrics collector", zap.Error(err))
			}
		}
	}

	s.running = false
	s.logger.Info("CloudAWSync service stopped")

	return nil
}

// IsRunning returns whether the service is running
func (s *Service) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

// GetStats returns service statistics
func (s *Service) GetStats() interfaces.SyncStats {
	if s.engine == nil {
		return interfaces.SyncStats{}
	}
	return s.engine.GetStats()
}

// GetMetrics returns service metrics
func (s *Service) GetMetrics() interfaces.Metrics {
	if s.metrics == nil {
		return interfaces.Metrics{}
	}
	return s.metrics.GetMetrics()
}

// AddDirectory adds a directory for synchronization
func (s *Service) AddDirectory(dir interfaces.SyncDirectory) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Add to config
	s.config.Directories = append(s.config.Directories, dir)

	// Add to engine if running
	if s.running && s.engine != nil {
		if engineImpl, ok := s.engine.(*engine.Engine); ok {
			engineImpl.AddDirectory(dir)
		}
	}

	s.logger.Info("Directory added for sync",
		zap.String("local_path", dir.LocalPath),
		zap.String("remote_path", dir.RemotePath),
		zap.String("sync_mode", string(dir.SyncMode)))

	return nil
}

// RemoveDirectory removes a directory from synchronization
func (s *Service) RemoveDirectory(localPath string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Remove from config
	for i, dir := range s.config.Directories {
		if dir.LocalPath == localPath {
			s.config.Directories = append(s.config.Directories[:i], s.config.Directories[i+1:]...)
			break
		}
	}

	s.logger.Info("Directory removed from sync",
		zap.String("local_path", localPath))

	return nil
}

// UpdateConfig updates the service configuration
func (s *Service) UpdateConfig(newConfig *config.Config) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Validate new config
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	oldConfig := s.config
	s.config = newConfig

	// If service is running, restart with new config
	if s.running {
		s.logger.Info("Restarting service with new configuration")

		// Stop current components
		if err := s.Stop(); err != nil {
			s.logger.Error("Failed to stop service for config update", zap.Error(err))
			s.config = oldConfig // Rollback
			return err
		}

		// Reinitialize components
		if err := s.initializeComponents(); err != nil {
			s.logger.Error("Failed to reinitialize components", zap.Error(err))
			s.config = oldConfig // Rollback
			return err
		}

		// Restart service
		if err := s.Start(); err != nil {
			s.logger.Error("Failed to restart service", zap.Error(err))
			s.config = oldConfig // Rollback
			return err
		}
	} else {
		// Just reinitialize components
		if err := s.initializeComponents(); err != nil {
			s.logger.Error("Failed to reinitialize components", zap.Error(err))
			s.config = oldConfig // Rollback
			return err
		}
	}

	s.logger.Info("Configuration updated successfully")
	return nil
}

// Wait waits for the service to stop
func (s *Service) Wait() {
	if s.ctx != nil {
		<-s.ctx.Done()
	}
}

// performInitialSync runs a one-time sync for all enabled directories on startup.
// This ensures that any files that already exist locally are uploaded if they
// are missing or outdated in the cloud.
func (s *Service) performInitialSync() {
	s.logger.Info("Performing initial startup sync for all configured directories")

	var wg sync.WaitGroup
	for _, dir := range s.config.Directories {
		if dir.Enabled {
			wg.Add(1)
			// Run each directory sync in its own goroutine for parallel execution
			go func(d interfaces.SyncDirectory) {
				defer wg.Done()
				s.logger.Info("Starting initial sync for directory", zap.String("local_path", d.LocalPath))
				if err := s.engine.Sync(s.ctx, d); err != nil {
					s.logger.Error("Initial sync failed for directory",
						zap.String("local_path", d.LocalPath),
						zap.Error(err))
				}
			}(dir)
		}
	}

	wg.Wait()
	s.logger.Info("Initial startup sync process completed for all directories")
}

// initializeComponents initializes all service components
func (s *Service) initializeComponents() error {
	var err error

	// Initialize cloud provider
	s.logger.Info("Creating cloud provider...")
	s.provider, err = s.createCloudProvider()
	if err != nil {
		s.logger.Error("Failed to create cloud provider", zap.Error(err))
		return fmt.Errorf("failed to create cloud provider: %w", err)
	}
	s.logger.Info("Cloud provider created successfully")

	// Initialize file watcher
	s.logger.Info("Creating file watcher...")
	s.watcher, err = s.createFileWatcher()
	if err != nil {
		s.logger.Error("Failed to create file watcher", zap.Error(err))
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	s.logger.Info("File watcher created successfully")

	// Initialize metrics collector
	s.logger.Info("Creating metrics collector...")
	s.metrics = s.createMetricsCollector()
	s.logger.Info("Metrics collector created successfully")

	// Initialize sync engine
	s.logger.Info("Creating sync engine...")
	s.engine = s.createSyncEngine()
	s.logger.Info("Sync engine created successfully")

	s.logger.Info("All components initialized successfully")
	return nil
}

// createCloudProvider creates the cloud provider based on configuration
func (s *Service) createCloudProvider() (interfaces.CloudProvider, error) {
	// For now, only S3 is supported
	s3Config := providers.S3Config{
		Region:               s.config.AWS.Region,
		Bucket:               s.config.AWS.S3Bucket,
		Prefix:               s.config.AWS.S3Prefix,
		Endpoint:             s.config.AWS.Endpoint,
		AccessKeyID:          s.config.AWS.AccessKeyID,
		SecretAccessKey:      s.config.AWS.SecretAccessKey,
		SessionToken:         s.config.AWS.SessionToken,
		ServerSideEncryption: s.config.Security.EncryptionEnabled,
	}

	provider, err := providers.NewS3Provider(s3Config, s.logger)
	if err != nil {
		return nil, err
	}

	s.logger.Info("S3 provider initialized",
		zap.String("region", s3Config.Region),
		zap.String("bucket", s3Config.Bucket))

	return provider, nil
}

// createFileWatcher creates the file watcher
func (s *Service) createFileWatcher() (interfaces.FileWatcher, error) {
	// Use batched watcher for better performance
	batchDelay := 2 * time.Second
	watcher, err := watcher.NewBatchedWatcher(s.logger, batchDelay)
	if err != nil {
		return nil, err
	}

	// Set filters from security config
	filters := append(s.config.Security.DeniedExtensions, ".tmp", ".swp", "~")
	watcher.SetFilters(filters)

	s.logger.Info("File watcher initialized with batching",
		zap.Duration("batch_delay", batchDelay))

	return watcher, nil
}

// createMetricsCollector creates the metrics collector
func (s *Service) createMetricsCollector() interfaces.MetricsCollector {
	if s.config.Metrics.Enabled {
		collector := metrics.NewPrometheusCollector(
			s.logger,
			s.config.Metrics.Port,
			s.config.Metrics.Path,
			s.config.Metrics.CollectInterval,
		)
		s.logger.Info("Prometheus metrics collector initialized",
			zap.Int("port", s.config.Metrics.Port),
			zap.String("path", s.config.Metrics.Path))
		return collector
	}

	collector := metrics.NewSimpleCollector(s.logger)
	s.logger.Info("Simple metrics collector initialized")
	return collector
}

// createSyncEngine creates the sync engine
func (s *Service) createSyncEngine() interfaces.SyncEngine {
	engine := engine.NewEngine(
		s.provider,
		s.watcher,
		s.metrics,
		s.logger,
		s.config.Performance.MaxConcurrentUploads,
		s.config.Performance.MaxConcurrentDownloads,
		s.config.Performance.RetryAttempts,
		s.config.Performance.RetryDelay,
	)

	s.logger.Info("Sync engine initialized",
		zap.Int("max_concurrent_uploads", s.config.Performance.MaxConcurrentUploads),
		zap.Int("max_concurrent_downloads", s.config.Performance.MaxConcurrentDownloads))

	return engine
}
