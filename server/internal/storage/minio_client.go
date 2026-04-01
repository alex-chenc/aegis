package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"server/config"
	"server/pkg/logger"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

type MinIOClient struct {
	client *minio.Client
	ctx    context.Context
}

func NewMinIOClient(cfg *config.MinIOConfig) (*MinIOClient, error) {
	ctx := context.Background()

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		logger.Error("failed to create MinIO client",
			zap.Error(err),
			zap.String("endpoint", cfg.Endpoint),
		)
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// 确保所有必需的 Bucket 存在
	buckets := cfg.Buckets
	for _, bucket := range buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			logger.Error("failed to check bucket",
				zap.Error(err),
				zap.String("bucket", bucket),
			)
			return nil, fmt.Errorf("failed to check bucket %s: %w", bucket, err)
		}
		if !exists {
			err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
			if err != nil {
				logger.Error("failed to create bucket",
					zap.Error(err),
					zap.String("bucket", bucket),
				)
				return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
			logger.Info("bucket created",
				zap.String("bucket", bucket),
			)
		} else {
			logger.Debug("bucket already exists",
				zap.String("bucket", bucket),
			)
		}
	}

	logger.Info("MinIO client initialized successfully",
		zap.String("endpoint", cfg.Endpoint),
		zap.Int("buckets", len(buckets)),
	)

	return &MinIOClient{
		client: client,
		ctx:    ctx,
	}, nil
}

func (m *MinIOClient) Client() *minio.Client {
	return m.client
}

func (m *MinIOClient) Context() context.Context {
	return m.ctx
}

// UploadFile uploads a file to the specified bucket
func (m *MinIOClient) UploadFile(bucket, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	info, err := m.client.PutObject(m.ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		logger.Error("failed to upload file",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	logger.Info("file uploaded successfully",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
		zap.Int64("size", info.Size),
		zap.String("etag", info.ETag),
	)

	return info.ETag, nil
}

// DownloadFile downloads a file from the specified bucket
func (m *MinIOClient) DownloadFile(bucket, objectName string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(m.ctx, bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		logger.Error("failed to download file",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	logger.Debug("file download initiated",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)

	return obj, nil
}

// DeleteFile deletes a file from the specified bucket
func (m *MinIOClient) DeleteFile(bucket, objectName string) error {
	err := m.client.RemoveObject(m.ctx, bucket, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		logger.Error("failed to delete file",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	logger.Info("file deleted successfully",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)

	return nil
}

// GetPresignedURL generates a presigned URL for downloading
func (m *MinIOClient) GetPresignedURL(bucket, objectName string, expiry time.Duration) (string, error) {
	url, err := m.client.PresignedGetObject(m.ctx, bucket, objectName, expiry, nil)
	if err != nil {
		logger.Error("failed to generate presigned URL",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
			zap.Duration("expiry", expiry),
		)
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	logger.Debug("presigned URL generated",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
		zap.Duration("expiry", expiry),
	)

	return url.String(), nil
}

// FileExists checks if a file exists in the specified bucket
func (m *MinIOClient) FileExists(bucket, objectName string) (bool, error) {
	_, err := m.client.StatObject(m.ctx, bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			logger.Debug("file does not exist",
				zap.String("bucket", bucket),
				zap.String("object", objectName),
			)
			return false, nil
		}
		logger.Error("failed to check file existence",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	logger.Debug("file exists",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)

	return true, nil
}

// GetFileSize gets the size of a file in bytes
func (m *MinIOClient) GetFileSize(bucket, objectName string) (int64, error) {
	info, err := m.client.StatObject(m.ctx, bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		logger.Error("failed to get file size",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return 0, fmt.Errorf("failed to get file size: %w", err)
	}

	return info.Size, nil
}

// ListFiles lists all files in a bucket with optional prefix
func (m *MinIOClient) ListFiles(bucket, prefix string, recursive bool) ([]string, error) {
	var files []string

	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	}

	for objInfo := range m.client.ListObjects(m.ctx, bucket, opts) {
		if objInfo.Err != nil {
			logger.Error("error listing files",
				zap.Error(objInfo.Err),
				zap.String("bucket", bucket),
				zap.String("prefix", prefix),
			)
			return nil, fmt.Errorf("error listing files: %w", objInfo.Err)
		}
		files = append(files, objInfo.Key)
	}

	logger.Debug("files listed",
		zap.String("bucket", bucket),
		zap.String("prefix", prefix),
		zap.Int("count", len(files)),
	)

	return files, nil
}

// GetContentType returns the content type based on file extension
func GetContentType(filename string) string {
	switch {
	case hasExtension(filename, ".pdf"):
		return "application/pdf"
	case hasExtension(filename, ".doc", ".docx"):
		return "application/msword"
	case hasExtension(filename, ".xls", ".xlsx"):
		return "application/vnd.ms-excel"
	case hasExtension(filename, ".yaml", ".yml"):
		return "application/x-yaml"
	case hasExtension(filename, ".txt"):
		return "text/plain"
	case hasExtension(filename, ".json"):
		return "application/json"
	case hasExtension(filename, ".sh"):
		return "application/x-sh"
	case hasExtension(filename, ".md"):
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

func hasExtension(filename string, extensions ...string) bool {
	for _, ext := range extensions {
		if len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext {
			return true
		}
	}
	return false
}
