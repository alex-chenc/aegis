package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"baseline-system/config"
	"baseline-system/pkg/logger"

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
	for _, bucket := range cfg.Buckets {
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
			logger.Info("bucket created successfully",
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
		zap.Int("buckets", len(cfg.Buckets)),
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
func (m *MinIOClient) UploadFile(bucket, objectName string, reader io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(m.ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		logger.Error("failed to upload file",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return err
	}

	logger.Info("file uploaded successfully",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
		zap.Int64("size", size),
	)
	return nil
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
		return nil, err
	}

	logger.Debug("file downloaded successfully",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)
	return obj, nil
}

// GetPresignedURL generates a presigned URL for downloading
func (m *MinIOClient) GetPresignedURL(bucket, objectName string, expiry time.Duration) (string, error) {
	url, err := m.client.PresignedGetObject(m.ctx, bucket, objectName, expiry, nil)
	if err != nil {
		logger.Error("failed to generate presigned URL",
			zap.Error(err),
			zap.String("bucket", bucket),
			zap.String("object", objectName),
		)
		return "", err
	}

	logger.Debug("presigned URL generated",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
		zap.Duration("expiry", expiry),
	)
	return url.String(), nil
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
		return err
	}

	logger.Info("file deleted successfully",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)
	return nil
}

// FileExists checks if a file exists in the specified bucket
func (m *MinIOClient) FileExists(bucket, objectName string) (bool, error) {
	_, err := m.client.StatObject(m.ctx, bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
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
		return false, err
	}

	logger.Debug("file exists",
		zap.String("bucket", bucket),
		zap.String("object", objectName),
	)
	return true, nil
}
