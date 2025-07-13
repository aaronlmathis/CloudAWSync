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

package interfaces

import (
	"context"
	"io"
	"time"
)

// CloudProvider defines the interface for cloud storage providers
type CloudProvider interface {
	// Upload uploads a file to the cloud storage
	Upload(ctx context.Context, key string, reader io.Reader, metadata FileMetadata) error

	// Download downloads a file from the cloud storage
	Download(ctx context.Context, key string) (io.ReadCloser, FileMetadata, error)

	// Delete removes a file from the cloud storage
	Delete(ctx context.Context, key string) error

	// List lists files in the cloud storage with optional prefix
	List(ctx context.Context, prefix string) ([]FileInfo, error)

	// GetMetadata retrieves metadata for a specific file
	GetMetadata(ctx context.Context, key string) (FileMetadata, error)

	// Exists checks if a file exists in the cloud storage
	Exists(ctx context.Context, key string) (bool, error)
}

// FileWatcher defines the interface for file system watchers
type FileWatcher interface {
	// Watch starts watching the specified directories
	Watch(ctx context.Context, dirs []string) (<-chan FileEvent, error)

	// Stop stops the file watcher
	Stop() error
}

// SyncEngine defines the interface for synchronization engines
type SyncEngine interface {
	// Sync performs synchronization for the specified directory
	Sync(ctx context.Context, dir SyncDirectory) error

	// Start starts the sync engine
	Start(ctx context.Context) error

	// Stop stops the sync engine
	Stop() error

	// GetStats returns synchronization statistics
	GetStats() SyncStats
}

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	// RecordBandwidth records bandwidth usage
	RecordBandwidth(bytes int64, direction string)

	// RecordFileOperation records file operation metrics
	RecordFileOperation(operation string, duration time.Duration, success bool)

	// RecordMemoryUsage records memory usage
	RecordMemoryUsage(bytes int64)

	// RecordCPUUsage records CPU usage
	RecordCPUUsage(percent float64)

	// GetMetrics returns current metrics
	GetMetrics() Metrics
}

// FileMetadata represents metadata for a file
type FileMetadata struct {
	Size        int64
	ModTime     time.Time
	MD5Hash     string
	ContentType string
	Permissions string
	Encrypted   bool
}

// FileInfo represents information about a file
type FileInfo struct {
	Key     string
	Size    int64
	ModTime time.Time
	MD5Hash string
	IsDir   bool
}

// FileEvent represents a file system event
type FileEvent struct {
	Path      string
	Operation string // create, modify, delete, move
	IsDir     bool
	Timestamp time.Time
}

// SyncDirectory represents a directory to be synchronized
type SyncDirectory struct {
	LocalPath  string
	RemotePath string
	SyncMode   SyncMode
	Schedule   string // cron expression for scheduled sync
	Recursive  bool
	Filters    []string // file patterns to include/exclude
	Enabled    bool
}

// SyncMode defines the synchronization mode
type SyncMode string

const (
	SyncModeRealtime  SyncMode = "realtime"  // sync on file changes
	SyncModeScheduled SyncMode = "scheduled" // sync on schedule
	SyncModeBoth      SyncMode = "both"      // both realtime and scheduled
)

// SyncStats represents synchronization statistics
type SyncStats struct {
	FilesUploaded     int64
	FilesDownloaded   int64
	FilesDeleted      int64
	BytesUploaded     int64
	BytesDownloaded   int64
	SyncErrors        int64
	LastSyncTime      time.Time
	ActiveDirectories int
}

// Metrics represents system and application metrics
type Metrics struct {
	BandwidthUp      int64   // bytes per second
	BandwidthDown    int64   // bytes per second
	MemoryUsage      int64   // bytes
	CPUUsage         float64 // percentage
	DiskUsage        int64   // bytes
	ActiveGoroutines int
	SyncStats        SyncStats
}

// SyncError represents a synchronization error
type SyncError struct {
	Path      string
	Operation string
	Error     error
	Timestamp time.Time
	Retries   int
}
