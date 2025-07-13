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

package engine

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"CloudAWSync/internal/interfaces"

	"go.uber.org/zap"
)

// Engine implements the SyncEngine interface
type Engine struct {
	provider interfaces.CloudProvider
	watcher  interfaces.FileWatcher
	metrics  interfaces.MetricsCollector
	logger   *zap.Logger

	// Configuration
	maxConcurrentUploads   int
	maxConcurrentDownloads int
	retryAttempts          int
	retryDelay             time.Duration

	// State
	directories   []interfaces.SyncDirectory
	uploadQueue   chan syncTask
	downloadQueue chan syncTask
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mutex         sync.RWMutex
	stats         interfaces.SyncStats
	running       bool
}

// syncTask represents a synchronization task
type syncTask struct {
	localPath  string
	remotePath string
	operation  string // upload, download, delete
	fileInfo   os.FileInfo
	metadata   interfaces.FileMetadata
}

// NewEngine creates a new sync engine
func NewEngine(
	provider interfaces.CloudProvider,
	watcher interfaces.FileWatcher,
	metrics interfaces.MetricsCollector,
	logger *zap.Logger,
	maxConcurrentUploads int,
	maxConcurrentDownloads int,
	retryAttempts int,
	retryDelay time.Duration,
) *Engine {
	return &Engine{
		provider:               provider,
		watcher:                watcher,
		metrics:                metrics,
		logger:                 logger,
		maxConcurrentUploads:   maxConcurrentUploads,
		maxConcurrentDownloads: maxConcurrentDownloads,
		retryAttempts:          retryAttempts,
		retryDelay:             retryDelay,
		uploadQueue:            make(chan syncTask, 100),
		downloadQueue:          make(chan syncTask, 100),
		stopChan:               make(chan struct{}),
	}
}

// Start starts the sync engine
func (e *Engine) Start(ctx context.Context) error {
	e.mutex.Lock()
	if e.running {
		e.mutex.Unlock()
		return fmt.Errorf("sync engine is already running")
	}
	e.running = true
	e.mutex.Unlock()

	e.logger.Info("Starting sync engine")

	// Start upload workers
	for i := 0; i < e.maxConcurrentUploads; i++ {
		e.wg.Add(1)
		go e.uploadWorker(ctx, i)
	}

	// Start download workers
	for i := 0; i < e.maxConcurrentDownloads; i++ {
		e.wg.Add(1)
		go e.downloadWorker(ctx, i)
	}

	// Start file watcher if we have realtime directories
	if e.hasRealtimeDirectories() {
		if err := e.startFileWatcher(ctx); err != nil {
			e.logger.Error("Failed to start file watcher", zap.Error(err))
			return err
		}
	}

	// Start scheduled sync if we have scheduled directories
	if e.hasScheduledDirectories() {
		e.wg.Add(1)
		go e.scheduledSyncWorker(ctx)
	}

	e.logger.Info("Sync engine started successfully")
	return nil
}

// Stop stops the sync engine
func (e *Engine) Stop() error {
	e.mutex.Lock()
	if !e.running {
		e.mutex.Unlock()
		return nil
	}
	e.running = false
	e.mutex.Unlock()

	e.logger.Info("Stopping sync engine")

	// Close stop channel
	close(e.stopChan)

	// Stop file watcher
	if e.watcher != nil {
		if err := e.watcher.Stop(); err != nil {
			e.logger.Error("Failed to stop file watcher", zap.Error(err))
		}
	}

	// Close queues
	close(e.uploadQueue)
	close(e.downloadQueue)

	// Wait for workers to finish
	e.wg.Wait()

	e.logger.Info("Sync engine stopped")
	return nil
}

// AddDirectory adds a directory for synchronization
func (e *Engine) AddDirectory(dir interfaces.SyncDirectory) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.directories = append(e.directories, dir)
	e.stats.ActiveDirectories = len(e.directories)

	e.logger.Info("Added directory for sync",
		zap.String("local_path", dir.LocalPath),
		zap.String("remote_path", dir.RemotePath),
		zap.String("sync_mode", string(dir.SyncMode)))
}

// Sync performs synchronization for the specified directory
func (e *Engine) Sync(ctx context.Context, dir interfaces.SyncDirectory) error {
	if !dir.Enabled {
		return nil
	}

	e.logger.Info("Starting sync for directory",
		zap.String("local_path", dir.LocalPath),
		zap.String("remote_path", dir.RemotePath))

	start := time.Now()
	err := e.syncDirectory(ctx, dir)
	duration := time.Since(start)

	e.metrics.RecordFileOperation("sync", duration, err == nil)

	if err != nil {
		e.logger.Error("Sync failed for directory",
			zap.String("local_path", dir.LocalPath),
			zap.Error(err))
		return err
	}

	e.logger.Info("Sync completed for directory",
		zap.String("local_path", dir.LocalPath),
		zap.Duration("duration", duration))

	return nil
}

// GetStats returns synchronization statistics
func (e *Engine) GetStats() interfaces.SyncStats {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.stats
}

// syncDirectory performs the actual synchronization for a directory
func (e *Engine) syncDirectory(ctx context.Context, dir interfaces.SyncDirectory) error {
	// Get local files
	localFiles, err := e.getLocalFiles(dir.LocalPath, dir.Recursive)
	if err != nil {
		return fmt.Errorf("failed to get local files: %w", err)
	}

	// Get remote files
	remoteFiles, err := e.provider.List(ctx, dir.RemotePath)
	if err != nil {
		return fmt.Errorf("failed to get remote files: %w", err)
	}

	// Create maps for efficient lookup
	localFileMap := make(map[string]os.FileInfo)
	for path, info := range localFiles {
		localFileMap[path] = info
	}

	remoteFileMap := make(map[string]interfaces.FileInfo)
	for _, info := range remoteFiles {
		remoteFileMap[info.Key] = info
	}

	// Determine what needs to be uploaded
	for localPath, localInfo := range localFiles {
		if !e.shouldSyncFile(localPath, dir.Filters) {
			continue
		}

		relativePath := e.getRelativePath(localPath, dir.LocalPath)
		remotePath := filepath.Join(dir.RemotePath, relativePath)

		remoteInfo, exists := remoteFileMap[remotePath]

		if !exists || e.needsUpload(localInfo, remoteInfo) {
			task := syncTask{
				localPath:  localPath,
				remotePath: remotePath,
				operation:  "upload",
				fileInfo:   localInfo,
			}

			select {
			case e.uploadQueue <- task:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// Determine what needs to be downloaded (if bidirectional sync)
	// For now, we'll focus on upload-only sync

	return nil
}

// uploadWorker processes upload tasks
func (e *Engine) uploadWorker(ctx context.Context, workerID int) {
	defer e.wg.Done()

	e.logger.Debug("Upload worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case task, ok := <-e.uploadQueue:
			if !ok {
				return
			}
			e.processUploadTask(ctx, task, workerID)
		}
	}
}

// downloadWorker processes download tasks
func (e *Engine) downloadWorker(ctx context.Context, workerID int) {
	defer e.wg.Done()

	e.logger.Debug("Download worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case task, ok := <-e.downloadQueue:
			if !ok {
				return
			}
			e.processDownloadTask(ctx, task, workerID)
		}
	}
}

// processUploadTask processes a single upload task
func (e *Engine) processUploadTask(ctx context.Context, task syncTask, workerID int) {
	start := time.Now()

	e.logger.Debug("Processing upload task",
		zap.Int("worker_id", workerID),
		zap.String("local_path", task.localPath),
		zap.String("remote_path", task.remotePath))

	var err error
	for attempt := 0; attempt <= e.retryAttempts; attempt++ {
		if attempt > 0 {
			e.logger.Warn("Retrying upload",
				zap.String("local_path", task.localPath),
				zap.Int("attempt", attempt))
			time.Sleep(e.retryDelay)
		}

		err = e.uploadFile(ctx, task)
		if err == nil {
			break
		}
	}

	duration := time.Since(start)
	e.metrics.RecordFileOperation("upload", duration, err == nil)

	if err != nil {
		e.logger.Error("Upload failed after retries",
			zap.String("local_path", task.localPath),
			zap.Error(err))
		e.incrementSyncErrors()
	} else {
		e.logger.Info("Upload completed",
			zap.String("local_path", task.localPath),
			zap.String("remote_path", task.remotePath),
			zap.Duration("duration", duration))
		e.incrementFilesUploaded()
	}
}

// processDownloadTask processes a single download task
func (e *Engine) processDownloadTask(ctx context.Context, task syncTask, workerID int) {
	start := time.Now()

	e.logger.Debug("Processing download task",
		zap.Int("worker_id", workerID),
		zap.String("local_path", task.localPath),
		zap.String("remote_path", task.remotePath))

	var err error
	for attempt := 0; attempt <= e.retryAttempts; attempt++ {
		if attempt > 0 {
			e.logger.Warn("Retrying download",
				zap.String("remote_path", task.remotePath),
				zap.Int("attempt", attempt))
			time.Sleep(e.retryDelay)
		}

		err = e.downloadFile(ctx, task)
		if err == nil {
			break
		}
	}

	duration := time.Since(start)
	e.metrics.RecordFileOperation("download", duration, err == nil)

	if err != nil {
		e.logger.Error("Download failed after retries",
			zap.String("remote_path", task.remotePath),
			zap.Error(err))
		e.incrementSyncErrors()
	} else {
		e.logger.Info("Download completed",
			zap.String("local_path", task.localPath),
			zap.String("remote_path", task.remotePath),
			zap.Duration("duration", duration))
		e.incrementFilesDownloaded()
	}
}

// uploadFile uploads a single file
func (e *Engine) uploadFile(ctx context.Context, task syncTask) error {
	file, err := os.Open(task.localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Calculate MD5 hash
	hasher := md5.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return fmt.Errorf("failed to calculate MD5: %w", err)
	}

	// Reset file pointer
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file pointer: %w", err)
	}

	metadata := interfaces.FileMetadata{
		Size:        size,
		ModTime:     task.fileInfo.ModTime(),
		MD5Hash:     fmt.Sprintf("%x", hasher.Sum(nil)),
		ContentType: e.getContentType(task.localPath),
		Permissions: task.fileInfo.Mode().String(),
	}

	// Record bandwidth
	e.metrics.RecordBandwidth(size, "upload")

	err = e.provider.Upload(ctx, task.remotePath, file, metadata)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

// downloadFile downloads a single file
func (e *Engine) downloadFile(ctx context.Context, task syncTask) error {
	reader, metadata, err := e.provider.Download(ctx, task.remotePath)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer reader.Close()

	// Create directory if it doesn't exist
	dir := filepath.Dir(task.localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(task.localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Calculate MD5 while copying
	hasher := md5.New()
	writer := io.MultiWriter(file, hasher)

	size, err := io.Copy(writer, reader)
	if err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	// Verify MD5 hash
	actualHash := fmt.Sprintf("%x", hasher.Sum(nil))
	if metadata.MD5Hash != "" && metadata.MD5Hash != actualHash {
		return fmt.Errorf("MD5 hash mismatch: expected %s, got %s",
			metadata.MD5Hash, actualHash)
	}

	// Set file modification time
	if !metadata.ModTime.IsZero() {
		if err := os.Chtimes(task.localPath, metadata.ModTime, metadata.ModTime); err != nil {
			e.logger.Warn("Failed to set file modification time",
				zap.String("path", task.localPath),
				zap.Error(err))
		}
	}

	// Record bandwidth
	e.metrics.RecordBandwidth(size, "download")

	return nil
}

// Helper methods for getting file information and managing state

func (e *Engine) getLocalFiles(rootPath string, recursive bool) (map[string]os.FileInfo, error) {
	files := make(map[string]os.FileInfo)

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		files[path] = info
		return nil
	}

	if recursive {
		err := filepath.Walk(rootPath, walkFn)
		return files, err
	}

	// Non-recursive: only process files in the root directory
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(rootPath, entry.Name())
			files[path] = info
		}
	}

	return files, nil
}

func (e *Engine) shouldSyncFile(path string, filters []string) bool {
	filename := filepath.Base(path)

	// Skip hidden files
	if strings.HasPrefix(filename, ".") {
		return false
	}

	// Apply filters
	for _, filter := range filters {
		if matched, _ := filepath.Match(filter, filename); matched {
			return false
		}
	}

	return true
}

func (e *Engine) getRelativePath(fullPath, rootPath string) string {
	relPath, err := filepath.Rel(rootPath, fullPath)
	if err != nil {
		return filepath.Base(fullPath)
	}
	return relPath
}

func (e *Engine) needsUpload(localInfo os.FileInfo, remoteInfo interfaces.FileInfo) bool {
	// Compare modification times and sizes
	return localInfo.ModTime().After(remoteInfo.ModTime) ||
		localInfo.Size() != remoteInfo.Size
}

func (e *Engine) getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	contentTypes := map[string]string{
		".txt":  "text/plain",
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".mp4":  "video/mp4",
		".mp3":  "audio/mpeg",
	}

	if contentType, ok := contentTypes[ext]; ok {
		return contentType
	}

	return "application/octet-stream"
}

func (e *Engine) hasRealtimeDirectories() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, dir := range e.directories {
		if dir.SyncMode == interfaces.SyncModeRealtime || dir.SyncMode == interfaces.SyncModeBoth {
			return true
		}
	}
	return false
}

func (e *Engine) hasScheduledDirectories() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, dir := range e.directories {
		if dir.SyncMode == interfaces.SyncModeScheduled || dir.SyncMode == interfaces.SyncModeBoth {
			return true
		}
	}
	return false
}

func (e *Engine) startFileWatcher(ctx context.Context) error {
	var dirs []string
	e.mutex.RLock()
	for _, dir := range e.directories {
		if (dir.SyncMode == interfaces.SyncModeRealtime || dir.SyncMode == interfaces.SyncModeBoth) && dir.Enabled {
			dirs = append(dirs, dir.LocalPath)
		}
	}
	e.mutex.RUnlock()

	if len(dirs) == 0 {
		return nil
	}

	events, err := e.watcher.Watch(ctx, dirs)
	if err != nil {
		return err
	}

	e.wg.Add(1)
	go e.handleFileEvents(ctx, events)

	return nil
}

func (e *Engine) handleFileEvents(ctx context.Context, events <-chan interfaces.FileEvent) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			e.processFileEvent(ctx, event)
		}
	}
}

func (e *Engine) processFileEvent(ctx context.Context, event interfaces.FileEvent) {
	if event.IsDir {
		return // Skip directory events
	}

	// Find matching directory configuration
	var matchedDir *interfaces.SyncDirectory
	e.mutex.RLock()
	for _, dir := range e.directories {
		if strings.HasPrefix(event.Path, dir.LocalPath) && dir.Enabled {
			if dir.SyncMode == interfaces.SyncModeRealtime || dir.SyncMode == interfaces.SyncModeBoth {
				matchedDir = &dir
				break
			}
		}
	}
	e.mutex.RUnlock()

	if matchedDir == nil {
		return
	}

	if !e.shouldSyncFile(event.Path, matchedDir.Filters) {
		return
	}

	switch event.Operation {
	case "create", "modify":
		// Queue for upload
		if info, err := os.Stat(event.Path); err == nil {
			relativePath := e.getRelativePath(event.Path, matchedDir.LocalPath)
			remotePath := filepath.Join(matchedDir.RemotePath, relativePath)

			task := syncTask{
				localPath:  event.Path,
				remotePath: remotePath,
				operation:  "upload",
				fileInfo:   info,
			}

			select {
			case e.uploadQueue <- task:
			case <-ctx.Done():
				return
			default:
				e.logger.Warn("Upload queue full, dropping task",
					zap.String("path", event.Path))
			}
		}
	case "delete":
		// Queue for deletion (if implemented)
		// For now, we'll skip deletion sync for safety
	}
}

func (e *Engine) scheduledSyncWorker(ctx context.Context) {
	defer e.wg.Done()

	// For simplicity, we'll use a basic interval-based scheduler
	// In a production system, you'd want to use a proper cron scheduler
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.runScheduledSync(ctx)
		}
	}
}

func (e *Engine) runScheduledSync(ctx context.Context) {
	e.mutex.RLock()
	var scheduledDirs []interfaces.SyncDirectory
	for _, dir := range e.directories {
		if (dir.SyncMode == interfaces.SyncModeScheduled || dir.SyncMode == interfaces.SyncModeBoth) && dir.Enabled {
			scheduledDirs = append(scheduledDirs, dir)
		}
	}
	e.mutex.RUnlock()

	for _, dir := range scheduledDirs {
		if err := e.Sync(ctx, dir); err != nil {
			e.logger.Error("Scheduled sync failed",
				zap.String("directory", dir.LocalPath),
				zap.Error(err))
		}
	}
}

func (e *Engine) incrementFilesUploaded() {
	e.mutex.Lock()
	e.stats.FilesUploaded++
	e.stats.LastSyncTime = time.Now()
	e.mutex.Unlock()
}

func (e *Engine) incrementFilesDownloaded() {
	e.mutex.Lock()
	e.stats.FilesDownloaded++
	e.stats.LastSyncTime = time.Now()
	e.mutex.Unlock()
}

func (e *Engine) incrementSyncErrors() {
	e.mutex.Lock()
	e.stats.SyncErrors++
	e.mutex.Unlock()
}
