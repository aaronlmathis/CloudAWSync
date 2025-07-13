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

package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"CloudAWSync/internal/interfaces"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// FSWatcher implements the FileWatcher interface using fsnotify
type FSWatcher struct {
	watcher   *fsnotify.Watcher
	logger    *zap.Logger
	eventChan chan interfaces.FileEvent
	done      chan struct{}
	filters   []string
}

// NewFSWatcher creates a new file system watcher
func NewFSWatcher(logger *zap.Logger) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &FSWatcher{
		watcher:   watcher,
		logger:    logger,
		eventChan: make(chan interfaces.FileEvent, 100), // buffered channel
		done:      make(chan struct{}),
	}, nil
}

// Watch starts watching the specified directories
func (w *FSWatcher) Watch(ctx context.Context, dirs []string) (<-chan interfaces.FileEvent, error) {
	// Add directories to watcher
	for _, dir := range dirs {
		if err := w.addDirectory(dir); err != nil {
			w.logger.Error("Failed to add directory to watcher",
				zap.String("directory", dir),
				zap.Error(err))
			continue
		}
		w.logger.Info("Started watching directory",
			zap.String("directory", dir))
	}

	// Start event processing goroutine
	go w.processEvents(ctx)

	return w.eventChan, nil
}

// Stop stops the file watcher
func (w *FSWatcher) Stop() error {
	close(w.done)

	if w.watcher != nil {
		if err := w.watcher.Close(); err != nil {
			w.logger.Error("Failed to close file watcher", zap.Error(err))
			return err
		}
	}

	close(w.eventChan)
	w.logger.Info("File watcher stopped")
	return nil
}

// SetFilters sets file filters for the watcher
func (w *FSWatcher) SetFilters(filters []string) {
	w.filters = filters
}

// addDirectory adds a directory to the watcher recursively
func (w *FSWatcher) addDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			w.logger.Warn("Error walking directory",
				zap.String("path", path),
				zap.Error(err))
			return nil // continue walking
		}

		if info.IsDir() {
			if err := w.watcher.Add(path); err != nil {
				w.logger.Warn("Failed to add directory to watcher",
					zap.String("path", path),
					zap.Error(err))
				return nil // continue walking
			}
			w.logger.Debug("Added directory to watcher",
				zap.String("path", path))
		}

		return nil
	})
}

// processEvents processes file system events
func (w *FSWatcher) processEvents(ctx context.Context) {
	defer w.logger.Info("Event processing stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("File watcher error", zap.Error(err))
		}
	}
}

// handleEvent handles a single file system event
func (w *FSWatcher) handleEvent(event fsnotify.Event) {
	// Skip if file matches filter patterns
	if w.shouldSkipFile(event.Name) {
		return
	}

	fileEvent := interfaces.FileEvent{
		Path:      event.Name,
		Timestamp: time.Now(),
	}

	// Check if path is a directory
	if stat, err := os.Stat(event.Name); err == nil {
		fileEvent.IsDir = stat.IsDir()
	}

	// Map fsnotify operations to our operations
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		fileEvent.Operation = "create"
		// If it's a new directory, start watching it
		if fileEvent.IsDir {
			if err := w.watcher.Add(event.Name); err != nil {
				w.logger.Warn("Failed to add new directory to watcher",
					zap.String("path", event.Name),
					zap.Error(err))
			} else {
				w.logger.Debug("Added new directory to watcher",
					zap.String("path", event.Name))
			}
		}
	case event.Op&fsnotify.Write == fsnotify.Write:
		fileEvent.Operation = "modify"
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		fileEvent.Operation = "delete"
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		fileEvent.Operation = "move"
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		// Skip chmod events as they don't affect file content
		return
	default:
		w.logger.Debug("Unknown file event",
			zap.String("path", event.Name),
			zap.String("op", event.Op.String()))
		return
	}

	w.logger.Debug("File event detected",
		zap.String("path", fileEvent.Path),
		zap.String("operation", fileEvent.Operation),
		zap.Bool("isDir", fileEvent.IsDir))

	// Send event to channel (non-blocking)
	select {
	case w.eventChan <- fileEvent:
	default:
		w.logger.Warn("Event channel full, dropping event",
			zap.String("path", fileEvent.Path),
			zap.String("operation", fileEvent.Operation))
	}
}

// shouldSkipFile checks if a file should be skipped based on filters
func (w *FSWatcher) shouldSkipFile(path string) bool {
	// Skip hidden files and directories
	filename := filepath.Base(path)
	if strings.HasPrefix(filename, ".") {
		return true
	}

	// Skip temporary files
	if strings.HasSuffix(filename, ".tmp") ||
		strings.HasSuffix(filename, ".swp") ||
		strings.HasSuffix(filename, "~") {
		return true
	}

	// Apply custom filters
	for _, filter := range w.filters {
		if matched, _ := filepath.Match(filter, filename); matched {
			return true
		}
	}

	return false
}

// BatchedWatcher wraps FSWatcher to provide batched events
type BatchedWatcher struct {
	watcher    *FSWatcher
	batchDelay time.Duration
	logger     *zap.Logger
}

// NewBatchedWatcher creates a new batched file watcher
func NewBatchedWatcher(logger *zap.Logger, batchDelay time.Duration) (*BatchedWatcher, error) {
	watcher, err := NewFSWatcher(logger)
	if err != nil {
		return nil, err
	}

	return &BatchedWatcher{
		watcher:    watcher,
		batchDelay: batchDelay,
		logger:     logger,
	}, nil
}

// Watch starts watching with event batching
func (b *BatchedWatcher) Watch(ctx context.Context, dirs []string) (<-chan interfaces.FileEvent, error) {
	rawEvents, err := b.watcher.Watch(ctx, dirs)
	if err != nil {
		return nil, err
	}

	batchedEvents := make(chan interfaces.FileEvent, 100)
	go b.batchEvents(ctx, rawEvents, batchedEvents)

	return batchedEvents, nil
}

// Stop stops the batched watcher
func (b *BatchedWatcher) Stop() error {
	return b.watcher.Stop()
}

// SetFilters sets file filters
func (b *BatchedWatcher) SetFilters(filters []string) {
	b.watcher.SetFilters(filters)
}

// batchEvents batches file events to reduce redundant operations
func (b *BatchedWatcher) batchEvents(ctx context.Context, input <-chan interfaces.FileEvent, output chan<- interfaces.FileEvent) {
	defer close(output)

	eventMap := make(map[string]interfaces.FileEvent)
	ticker := time.NewTicker(b.batchDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-input:
			if !ok {
				// Flush remaining events
				b.flushEvents(eventMap, output)
				return
			}
			// Store latest event for each path
			eventMap[event.Path] = event
		case <-ticker.C:
			// Flush batched events
			b.flushEvents(eventMap, output)
			eventMap = make(map[string]interfaces.FileEvent)
		}
	}
}

// flushEvents sends all batched events
func (b *BatchedWatcher) flushEvents(eventMap map[string]interfaces.FileEvent, output chan<- interfaces.FileEvent) {
	for _, event := range eventMap {
		select {
		case output <- event:
		default:
			b.logger.Warn("Batched event channel full, dropping event",
				zap.String("path", event.Path),
				zap.String("operation", event.Operation))
		}
	}
}
