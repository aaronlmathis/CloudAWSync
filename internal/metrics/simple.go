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

package metrics

import (
	"context"
	"sync"
	"time"

	"CloudAWSync/internal/interfaces"

	"go.uber.org/zap"
)

// SimpleCollector is a basic metrics collector without Prometheus
type SimpleCollector struct {
	mutex   sync.RWMutex
	metrics interfaces.Metrics
	logger  *zap.Logger
}

// NewSimpleCollector creates a new simple metrics collector
func NewSimpleCollector(logger *zap.Logger) *SimpleCollector {
	return &SimpleCollector{
		logger: logger,
	}
}

// Start implements MetricsCollector interface (no-op for simple collector)
func (s *SimpleCollector) Start(ctx context.Context) error {
	s.logger.Info("Simple metrics collector started")
	return nil
}

// Stop implements MetricsCollector interface (no-op for simple collector)
func (s *SimpleCollector) Stop() error {
	s.logger.Info("Simple metrics collector stopped")
	return nil
}

// RecordBandwidth records bandwidth usage
func (s *SimpleCollector) RecordBandwidth(bytes int64, direction string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	switch direction {
	case "up", "upload":
		s.metrics.BandwidthUp += bytes
	case "down", "download":
		s.metrics.BandwidthDown += bytes
	}
}

// RecordFileOperation records file operation metrics
func (s *SimpleCollector) RecordFileOperation(operation string, duration time.Duration, success bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if success {
		switch operation {
		case "upload":
			s.metrics.SyncStats.FilesUploaded++
		case "download":
			s.metrics.SyncStats.FilesDownloaded++
		case "delete":
			s.metrics.SyncStats.FilesDeleted++
		}
		s.metrics.SyncStats.LastSyncTime = time.Now()
	} else {
		s.metrics.SyncStats.SyncErrors++
	}
}

// RecordMemoryUsage records memory usage
func (s *SimpleCollector) RecordMemoryUsage(bytes int64) {
	s.mutex.Lock()
	s.metrics.MemoryUsage = bytes
	s.mutex.Unlock()
}

// RecordCPUUsage records CPU usage
func (s *SimpleCollector) RecordCPUUsage(percent float64) {
	s.mutex.Lock()
	s.metrics.CPUUsage = percent
	s.mutex.Unlock()
}

// GetMetrics returns current metrics
func (s *SimpleCollector) GetMetrics() interfaces.Metrics {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.metrics
}
