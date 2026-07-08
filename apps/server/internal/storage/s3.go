package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
)

func (c *Client) s3PresignPut(ctx context.Context, cfg systemcodec.StorageProfile, _, key, contentType string, expires time.Duration) (string, error) {
	mc, err := newMinio(cfg.S3)
	if err != nil {
		return "", err
	}
	u, err := mc.Presign(ctx, "PUT", cfg.S3.Bucket, key, expires, url.Values{
		"Content-Type": {contentType},
	})
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return u.String(), nil
}

func (c *Client) s3ResolveURL(ctx context.Context, cfg systemcodec.StorageProfile, _, key string, expires time.Duration) (string, error) {
	if cfg.S3.PublicBaseUrl != "" {
		return strings.TrimSuffix(cfg.S3.PublicBaseUrl, "/") + "/" + strings.TrimPrefix(key, "/"), nil
	}
	mc, err := newMinio(cfg.S3)
	if err != nil {
		return "", err
	}
	u, err := mc.PresignedGetObject(ctx, cfg.S3.Bucket, key, expires, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return u.String(), nil
}

// s3Delete removes an object. S3 DELETE is idempotent — removing a key that is
// already gone succeeds — so the sweep can safely re-run after a crash.
func (c *Client) s3Delete(ctx context.Context, cfg systemcodec.StorageProfile, _, key string) error {
	mc, err := newMinio(cfg.S3)
	if err != nil {
		return err
	}
	if err := mc.RemoveObject(ctx, cfg.S3.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

// s3Test verifies the config end-to-end by checking the bucket exists — the
// cheapest call that exercises endpoint, credentials and bucket.
func (c *Client) s3Test(ctx context.Context, cfg systemcodec.StorageProfile) error {
	mc, err := newMinio(cfg.S3)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ok, err := mc.BucketExists(ctx, cfg.S3.Bucket)
	if err != nil {
		return fmt.Errorf("connect to bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", cfg.S3.Bucket)
	}
	return nil
}

func newMinio(cfg systemcodec.S3StorageConfig) (*minio.Client, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyId == "" {
		return nil, ErrNotConfigured
	}
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyId, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSsl,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}
	return mc, nil
}
