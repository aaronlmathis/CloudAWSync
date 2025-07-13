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

package providers

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"CloudAWSync/internal/interfaces"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// S3Provider implements the CloudProvider interface for AWS S3
type S3Provider struct {
	client *s3.Client
	bucket string
	prefix string
	logger *zap.Logger
	config S3Config
}

// S3Config holds S3-specific configuration
type S3Config struct {
	Region               string
	Bucket               string
	Prefix               string
	Endpoint             string
	AccessKeyID          string
	SecretAccessKey      string
	SessionToken         string
	StorageClass         string
	ServerSideEncryption bool
}

// NewS3Provider creates a new S3 provider
func NewS3Provider(cfg S3Config, logger *zap.Logger) (*S3Provider, error) {
	awsConfig, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Override credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		awsConfig.Credentials = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     cfg.AccessKeyID,
				SecretAccessKey: cfg.SecretAccessKey,
				SessionToken:    cfg.SessionToken,
			}, nil
		})
	}

	// Configure custom endpoint if provided
	if cfg.Endpoint != "" {
		awsConfig.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: cfg.Endpoint}, nil
			})
	}

	client := s3.NewFromConfig(awsConfig)

	provider := &S3Provider{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
		logger: logger,
		config: cfg,
	}

	// Verify bucket access
	if err := provider.verifyBucketAccess(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to verify bucket access: %w", err)
	}

	return provider, nil
}

// Upload uploads a file to S3
func (s *S3Provider) Upload(ctx context.Context, key string, reader io.Reader, metadata interfaces.FileMetadata) error {
	key = s.addPrefix(key)

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(metadata.Size), // Explicitly set Content-Length
	}

	// Set metadata
	input.Metadata = map[string]string{
		"original-path": key,
		"upload-time":   time.Now().UTC().Format(time.RFC3339),
		"content-type":  metadata.ContentType,
		"permissions":   metadata.Permissions,
		"md5-hash":      metadata.MD5Hash, // Store hex-encoded hash in metadata
	}

	// Set content type if available
	if metadata.ContentType != "" {
		input.ContentType = aws.String(metadata.ContentType)
	}

	// Convert hex MD5 hash to base64 for S3 ContentMD5 header
	if metadata.MD5Hash != "" {
		hexBytes, err := hex.DecodeString(metadata.MD5Hash)
		if err != nil {
			s.logger.Warn("Invalid MD5 hash format, skipping ContentMD5 header",
				zap.String("hash", metadata.MD5Hash),
				zap.Error(err))
		} else {
			// Convert to base64
			base64Hash := base64.StdEncoding.EncodeToString(hexBytes)
			input.ContentMD5 = aws.String(base64Hash)
		}
	}

	// Set storage class if configured
	if s.config.StorageClass != "" {
		input.StorageClass = types.StorageClass(s.config.StorageClass)
	}

	// Enable server-side encryption if configured
	if s.config.ServerSideEncryption {
		input.ServerSideEncryption = types.ServerSideEncryptionAes256
	}

	// Perform upload
	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		s.logger.Error("Failed to upload file to S3",
			zap.String("key", key),
			zap.Int64("size", metadata.Size),
			zap.Error(err))
		return fmt.Errorf("failed to upload file: %w", err)
	}

	s.logger.Info("Successfully uploaded file to S3",
		zap.String("key", key),
		zap.Int64("size", metadata.Size),
		zap.String("md5", metadata.MD5Hash))

	return nil
}

// Download downloads a file from S3
func (s *S3Provider) Download(ctx context.Context, key string) (io.ReadCloser, interfaces.FileMetadata, error) {
	key = s.addPrefix(key)

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		s.logger.Error("Failed to download file from S3",
			zap.String("key", key),
			zap.Error(err))
		return nil, interfaces.FileMetadata{}, fmt.Errorf("failed to download file: %w", err)
	}

	metadata := interfaces.FileMetadata{
		Size:        aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType),
	}

	if result.LastModified != nil {
		metadata.ModTime = *result.LastModified
	}

	// Extract custom metadata
	if result.Metadata != nil {
		if perms, ok := result.Metadata["permissions"]; ok {
			metadata.Permissions = perms
		}
	}

	s.logger.Info("Successfully downloaded file from S3",
		zap.String("key", key),
		zap.Int64("size", metadata.Size))

	return result.Body, metadata, nil
}

// Delete removes a file from S3
func (s *S3Provider) Delete(ctx context.Context, key string) error {
	key = s.addPrefix(key)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(ctx, input)
	if err != nil {
		s.logger.Error("Failed to delete file from S3",
			zap.String("key", key),
			zap.Error(err))
		return fmt.Errorf("failed to delete file: %w", err)
	}

	s.logger.Info("Successfully deleted file from S3",
		zap.String("key", key))

	return nil
}

// List lists files in S3 with optional prefix
func (s *S3Provider) List(ctx context.Context, prefix string) ([]interfaces.FileInfo, error) {
	fullPrefix := s.addPrefix(prefix)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	}

	var files []interfaces.FileInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			s.logger.Error("Failed to list files from S3",
				zap.String("prefix", fullPrefix),
				zap.Error(err))
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		for _, obj := range page.Contents {
			key := s.removePrefix(aws.ToString(obj.Key))

			fileInfo := interfaces.FileInfo{
				Key:   key,
				Size:  aws.ToInt64(obj.Size),
				IsDir: strings.HasSuffix(key, "/"),
			}

			if obj.LastModified != nil {
				fileInfo.ModTime = *obj.LastModified
			}

			files = append(files, fileInfo)
		}
	}

	s.logger.Debug("Listed files from S3",
		zap.String("prefix", fullPrefix),
		zap.Int("count", len(files)))

	return files, nil
}

// GetMetadata retrieves metadata for a specific file
func (s *S3Provider) GetMetadata(ctx context.Context, key string) (interfaces.FileMetadata, error) {
	key = s.addPrefix(key)

	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.HeadObject(ctx, input)
	if err != nil {
		s.logger.Error("Failed to get metadata from S3",
			zap.String("key", key),
			zap.Error(err))
		return interfaces.FileMetadata{}, fmt.Errorf("failed to get metadata: %w", err)
	}

	metadata := interfaces.FileMetadata{
		Size:        aws.ToInt64(result.ContentLength),
		ContentType: aws.ToString(result.ContentType),
	}

	if result.LastModified != nil {
		metadata.ModTime = *result.LastModified
	}

	// Extract custom metadata
	if result.Metadata != nil {
		if perms, ok := result.Metadata["permissions"]; ok {
			metadata.Permissions = perms
		}
	}

	// Get ETag as MD5 hash (for non-multipart uploads)
	if result.ETag != nil {
		etag := strings.Trim(aws.ToString(result.ETag), `"`)
		if !strings.Contains(etag, "-") { // Simple upload, ETag is MD5
			metadata.MD5Hash = etag
		}
	}

	return metadata, nil
}

// Exists checks if a file exists in S3
func (s *S3Provider) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.GetMetadata(ctx, key)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// verifyBucketAccess verifies that we can access the S3 bucket
func (s *S3Provider) verifyBucketAccess(ctx context.Context) error {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	}

	_, err := s.client.HeadBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("cannot access bucket %s: %w", s.bucket, err)
	}

	s.logger.Info("Successfully verified S3 bucket access",
		zap.String("bucket", s.bucket))

	return nil
}

// addPrefix adds the configured prefix to a key
func (s *S3Provider) addPrefix(key string) string {
	if s.prefix == "" {
		return key
	}
	return filepath.Join(s.prefix, key)
}

// removePrefix removes the configured prefix from a key
func (s *S3Provider) removePrefix(key string) string {
	if s.prefix == "" {
		return key
	}
	if strings.HasPrefix(key, s.prefix) {
		return strings.TrimPrefix(key, s.prefix)
	}
	return key
}
