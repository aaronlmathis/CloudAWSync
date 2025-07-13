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

package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CalculateMD5 calculates the MD5 hash of a file
func CalculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// CalculateSHA256 calculates the SHA256 hash of a file
func CalculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// FileExists checks if a file exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// IsDirectory checks if a path is a directory
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// GetFileSize returns the size of a file in bytes
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// SanitizePath sanitizes a file path for use as a remote key
func SanitizePath(path string) string {
	// Replace backslashes with forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	// Remove leading slashes
	path = strings.TrimPrefix(path, "/")

	// Clean the path
	path = filepath.Clean(path)

	return path
}

// FormatBytes formats bytes into human-readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetContentType returns the content type based on file extension
func GetContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	contentTypes := map[string]string{
		".txt":  "text/plain",
		".html": "text/html",
		".htm":  "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".flac": "audio/flac",
		".aac":  "audio/aac",
		".ogg":  "audio/ogg",
		".zip":  "application/zip",
		".rar":  "application/x-rar-compressed",
		".7z":   "application/x-7z-compressed",
		".tar":  "application/x-tar",
		".gz":   "application/gzip",
		".bz2":  "application/x-bzip2",
	}

	if contentType, ok := contentTypes[ext]; ok {
		return contentType
	}

	return "application/octet-stream"
}

// IsBinaryFile checks if a file is likely binary based on its content
func IsBinaryFile(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read first 512 bytes
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}

	// Check for null bytes (common in binary files)
	for i := 0; i < n; i++ {
		if buffer[i] == 0 {
			return true, nil
		}
	}

	return false, nil
}

// ValidateFilePath validates that a file path is safe
func ValidateFilePath(path string) error {
	// Check for dangerous path elements
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains '..' which is not allowed")
	}

	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute paths are not allowed")
	}

	// Check for reserved names on Windows
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4",
		"COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4",
		"LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}

	pathUpper := strings.ToUpper(filepath.Base(path))
	for _, name := range reserved {
		if pathUpper == name || strings.HasPrefix(pathUpper, name+".") {
			return fmt.Errorf("path contains reserved name: %s", name)
		}
	}

	return nil
}

// CreateTempFile creates a temporary file with the given prefix
func CreateTempFile(prefix string) (*os.File, error) {
	return os.CreateTemp("", prefix+"_*.tmp")
}

// AtomicWrite writes data to a file atomically by writing to a temp file first
func AtomicWrite(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	// Create temp file in the same directory
	tempFile, err := os.CreateTemp(dir, ".tmp_"+filepath.Base(filename)+"_")
	if err != nil {
		return err
	}

	tempName := tempFile.Name()

	// Clean up temp file on error
	defer func() {
		if err != nil {
			os.Remove(tempName)
		}
	}()

	// Write data to temp file
	if _, err = tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}

	// Set permissions
	if err = tempFile.Chmod(perm); err != nil {
		tempFile.Close()
		return err
	}

	// Close temp file
	if err = tempFile.Close(); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tempName, filename)
}
