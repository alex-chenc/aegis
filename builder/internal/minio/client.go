package minio

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	client *minio.Client
}

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

func NewClient(cfg Config) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Client{client: client}, nil
}

func (c *Client) UploadFile(ctx context.Context, bucket, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	_, err = c.client.PutObject(ctx, bucket, key, f, stat.Size(), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	return nil
}

func (c *Client) DownloadFile(ctx context.Context, bucket, key, destPath string) error {
	obj, err := c.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := obj.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write: %w", writeErr)
			}
		}
		if readErr != nil {
			break
		}
	}
	return nil
}

func (c *Client) GetDownloadURL(ctx context.Context, bucket, key string) (string, error) {
	url, err := c.client.PresignedGetObject(ctx, bucket, key, 24*time.Hour, nil)
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
	}
	return url.String(), nil
}
