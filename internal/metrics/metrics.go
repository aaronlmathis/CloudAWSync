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
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"CloudAWSync/internal/interfaces"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
)

// PrometheusCollector implements the MetricsCollector interface using Prometheus metrics
type PrometheusCollector struct {
	logger *zap.Logger
	server *http.Server

	// Prometheus metrics
	bandwidthUp       prometheus.Counter
	bandwidthDown     prometheus.Counter
	fileOperations    *prometheus.CounterVec
	operationDuration *prometheus.HistogramVec
	memoryUsage       prometheus.Gauge
	cpuUsage          prometheus.Gauge
	diskUsage         prometheus.Gauge
	activeGoroutines  prometheus.Gauge

	// Sync statistics
	filesUploaded   prometheus.Counter
	filesDownloaded prometheus.Counter
	filesDeleted    prometheus.Counter
	bytesUploaded   prometheus.Counter
	bytesDownloaded prometheus.Counter
	syncErrors      prometheus.Counter
	lastSyncTime    prometheus.Gauge

	// Internal state
	mutex           sync.RWMutex
	currentMetrics  interfaces.Metrics
	collectInterval time.Duration
	stopChan        chan struct{}
}

// NewPrometheusCollector creates a new Prometheus metrics collector
func NewPrometheusCollector(logger *zap.Logger, port int, path string, collectInterval time.Duration) *PrometheusCollector {
	collector := &PrometheusCollector{
		logger:          logger,
		collectInterval: collectInterval,
		stopChan:        make(chan struct{}),
	}

	// Initialize Prometheus metrics
	collector.initMetrics()

	// Setup HTTP server for metrics endpoint
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())

	collector.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return collector
}

// initMetrics initializes Prometheus metrics
func (p *PrometheusCollector) initMetrics() {
	p.bandwidthUp = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_bandwidth_up_bytes_total",
		Help: "Total bytes uploaded",
	})

	p.bandwidthDown = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_bandwidth_down_bytes_total",
		Help: "Total bytes downloaded",
	})

	p.fileOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudawsync_file_operations_total",
			Help: "Total file operations by type and status",
		},
		[]string{"operation", "status"},
	)

	p.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudawsync_operation_duration_seconds",
			Help:    "Duration of file operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	p.memoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudawsync_memory_usage_bytes",
		Help: "Current memory usage in bytes",
	})

	p.cpuUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudawsync_cpu_usage_percent",
		Help: "Current CPU usage percentage",
	})

	p.diskUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudawsync_disk_usage_bytes",
		Help: "Current disk usage in bytes",
	})

	p.activeGoroutines = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudawsync_active_goroutines",
		Help: "Number of active goroutines",
	})

	p.filesUploaded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_files_uploaded_total",
		Help: "Total files uploaded",
	})

	p.filesDownloaded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_files_downloaded_total",
		Help: "Total files downloaded",
	})

	p.filesDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_files_deleted_total",
		Help: "Total files deleted",
	})

	p.bytesUploaded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_bytes_uploaded_total",
		Help: "Total bytes uploaded",
	})

	p.bytesDownloaded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_bytes_downloaded_total",
		Help: "Total bytes downloaded",
	})

	p.syncErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudawsync_sync_errors_total",
		Help: "Total synchronization errors",
	})

	p.lastSyncTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudawsync_last_sync_timestamp",
		Help: "Timestamp of last successful sync",
	})

	// Register metrics with Prometheus
	prometheus.MustRegister(
		p.bandwidthUp,
		p.bandwidthDown,
		p.fileOperations,
		p.operationDuration,
		p.memoryUsage,
		p.cpuUsage,
		p.diskUsage,
		p.activeGoroutines,
		p.filesUploaded,
		p.filesDownloaded,
		p.filesDeleted,
		p.bytesUploaded,
		p.bytesDownloaded,
		p.syncErrors,
		p.lastSyncTime,
	)
}

// Start starts the metrics collector
func (p *PrometheusCollector) Start(ctx context.Context) error {
	// Start metrics collection goroutine
	go p.collectSystemMetrics(ctx)

	// Start HTTP server
	go func() {
		p.logger.Info("Starting metrics server",
			zap.String("address", p.server.Addr))
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the metrics collector
func (p *PrometheusCollector) Stop() error {
	close(p.stopChan)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		p.logger.Error("Failed to shutdown metrics server", zap.Error(err))
		return err
	}

	p.logger.Info("Metrics collector stopped")
	return nil
}

// RecordBandwidth records bandwidth usage
func (p *PrometheusCollector) RecordBandwidth(bytes int64, direction string) {
	switch direction {
	case "up", "upload":
		p.bandwidthUp.Add(float64(bytes))
		p.mutex.Lock()
		p.currentMetrics.BandwidthUp += bytes
		p.mutex.Unlock()
	case "down", "download":
		p.bandwidthDown.Add(float64(bytes))
		p.mutex.Lock()
		p.currentMetrics.BandwidthDown += bytes
		p.mutex.Unlock()
	}
}

// RecordFileOperation records file operation metrics
func (p *PrometheusCollector) RecordFileOperation(operation string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}

	p.fileOperations.WithLabelValues(operation, status).Inc()
	p.operationDuration.WithLabelValues(operation).Observe(duration.Seconds())

	// Update sync stats
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if success {
		switch operation {
		case "upload":
			p.filesUploaded.Inc()
			p.currentMetrics.SyncStats.FilesUploaded++
		case "download":
			p.filesDownloaded.Inc()
			p.currentMetrics.SyncStats.FilesDownloaded++
		case "delete":
			p.filesDeleted.Inc()
			p.currentMetrics.SyncStats.FilesDeleted++
		}
		p.currentMetrics.SyncStats.LastSyncTime = time.Now()
		p.lastSyncTime.SetToCurrentTime()
	} else {
		p.syncErrors.Inc()
		p.currentMetrics.SyncStats.SyncErrors++
	}
}

// RecordMemoryUsage records memory usage
func (p *PrometheusCollector) RecordMemoryUsage(bytes int64) {
	p.memoryUsage.Set(float64(bytes))
	p.mutex.Lock()
	p.currentMetrics.MemoryUsage = bytes
	p.mutex.Unlock()
}

// RecordCPUUsage records CPU usage
func (p *PrometheusCollector) RecordCPUUsage(percent float64) {
	p.cpuUsage.Set(percent)
	p.mutex.Lock()
	p.currentMetrics.CPUUsage = percent
	p.mutex.Unlock()
}

// RecordBytesTransferred records bytes transferred for sync operations
func (p *PrometheusCollector) RecordBytesTransferred(bytes int64, direction string) {
	switch direction {
	case "up", "upload":
		p.bytesUploaded.Add(float64(bytes))
		p.mutex.Lock()
		p.currentMetrics.SyncStats.BytesUploaded += bytes
		p.mutex.Unlock()
	case "down", "download":
		p.bytesDownloaded.Add(float64(bytes))
		p.mutex.Lock()
		p.currentMetrics.SyncStats.BytesDownloaded += bytes
		p.mutex.Unlock()
	}
}

// GetMetrics returns current metrics
func (p *PrometheusCollector) GetMetrics() interfaces.Metrics {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.currentMetrics
}

// collectSystemMetrics collects system metrics periodically
func (p *PrometheusCollector) collectSystemMetrics(ctx context.Context) {
	ticker := time.NewTicker(p.collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.updateSystemMetrics()
		}
	}
}

// updateSystemMetrics updates system metrics
func (p *PrometheusCollector) updateSystemMetrics() {
	// Memory usage
	if memInfo, err := mem.VirtualMemory(); err == nil {
		p.RecordMemoryUsage(int64(memInfo.Used))
	}

	// CPU usage
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		p.RecordCPUUsage(cpuPercent[0])
	}

	// Disk usage
	if diskInfo, err := disk.Usage("/"); err == nil {
		p.diskUsage.Set(float64(diskInfo.Used))
		p.mutex.Lock()
		p.currentMetrics.DiskUsage = int64(diskInfo.Used)
		p.mutex.Unlock()
	}

	// Goroutines
	goroutines := runtime.NumGoroutine()
	p.activeGoroutines.Set(float64(goroutines))
	p.mutex.Lock()
	p.currentMetrics.ActiveGoroutines = goroutines
	p.mutex.Unlock()
}

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
