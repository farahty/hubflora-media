package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/farahty/hubflora-media/internal/config"
)

// S3Client wraps minio-go for S3-compatible storage operations.
type S3Client struct {
	client *minio.Client
	cfg    *config.Config
}

// NewS3Client creates a new S3-compatible storage client.
func NewS3Client(cfg *config.Config) (*S3Client, error) {
	client, err := minio.New(cfg.MinioAddr(), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	return &S3Client{client: client, cfg: cfg}, nil
}

// EnsureBucket creates the default bucket if it doesn't exist.
func (s *S3Client) EnsureBucket(ctx context.Context) error {
	bucket := s.cfg.MinioDefaultBucket
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("failed to create bucket %q: %w", bucket, err)
	}

	// Set public-read policy
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, bucket)

	if err := s.client.SetBucketPolicy(ctx, bucket, policy); err != nil {
		slog.Warn("failed to set bucket policy", "bucket", bucket, "error", err)
	}

	return nil
}

// Upload stores a file in S3.
func (s *S3Client) Upload(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}
	_, err := s.client.PutObject(ctx, bucket, objectKey, reader, size, opts)
	if err != nil {
		return fmt.Errorf("failed to upload %q: %w", objectKey, err)
	}
	return nil
}

// Download retrieves a file from S3 as an io.ReadCloser.
func (s *S3Client) Download(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %q: %w", objectKey, err)
	}
	return obj, nil
}

// GetBuffer reads the full content of an object into a byte slice.
func (s *S3Client) GetBuffer(ctx context.Context, bucket, objectKey string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %q: %w", objectKey, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read object %q: %w", objectKey, err)
	}
	return data, nil
}

// Delete removes an object from S3.
func (s *S3Client) Delete(ctx context.Context, bucket, objectKey string) error {
	err := s.client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete %q: %w", objectKey, err)
	}
	return nil
}

// DeletePrefix deletes all objects under a given prefix.
func (s *S3Client) DeletePrefix(ctx context.Context, bucket, prefix string) error {
	objectsCh := s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objectsCh {
		if obj.Err != nil {
			return fmt.Errorf("error listing objects: %w", obj.Err)
		}
		if err := s.client.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			slog.Warn("failed to delete object", "key", obj.Key, "error", err)
		}
	}
	return nil
}

// GetPublicURL returns the public URL for an object.
func (s *S3Client) GetPublicURL(bucket, objectKey string) string {
	if s.cfg.MinioUseCDN && s.cfg.MinioCDNDomain != "" {
		return fmt.Sprintf("https://%s/%s", s.cfg.MinioCDNDomain, objectKey)
	}

	scheme := "http"
	if s.cfg.MinioUseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, s.cfg.MinioAddr(), bucket, objectKey)
}

// PresignedGetURL generates a pre-signed download URL.
func (s *S3Client) PresignedGetURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	u, err := s.client.PresignedGetObject(ctx, bucket, objectKey, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return u.String(), nil
}

// PresignedPutURL generates a pre-signed upload URL.
func (s *S3Client) PresignedPutURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, bucket, objectKey, expiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}
	return u.String(), nil
}

// Stat retrieves object metadata without downloading.
func (s *S3Client) Stat(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error) {
	return s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
}

// ListObjects lists objects with a given prefix.
func (s *S3Client) ListObjects(ctx context.Context, bucket, prefix string) ([]minio.ObjectInfo, error) {
	var objects []minio.ObjectInfo
	objectsCh := s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for obj := range objectsCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("error listing objects: %w", obj.Err)
		}
		objects = append(objects, obj)
	}
	return objects, nil
}
