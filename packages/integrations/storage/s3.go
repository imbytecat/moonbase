package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
)

func s3PresignPut(_ LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, _, key, contentType string, expires time.Duration) (string, error) {
	mc, err := newMinio(cfg.Config)
	if err != nil {
		return "", err
	}
	u, err := mc.Presign(ctx, "PUT", cfgStr(cfg.Config, "bucket"), key, expires, url.Values{
		"Content-Type": {contentType},
	})
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return u.String(), nil
}

func s3ResolveURL(_ LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, _, key string, expires time.Duration) (string, error) {
	if publicBaseURL := cfgStr(cfg.Config, "publicBaseUrl"); publicBaseURL != "" {
		return strings.TrimSuffix(publicBaseURL, "/") + "/" + strings.TrimPrefix(key, "/"), nil
	}
	mc, err := newMinio(cfg.Config)
	if err != nil {
		return "", err
	}
	u, err := mc.PresignedGetObject(ctx, cfgStr(cfg.Config, "bucket"), key, expires, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return u.String(), nil
}

// s3Delete removes an object. S3 DELETE is idempotent — removing a key that is
// already gone succeeds — so the sweep can safely re-run after a crash.
func s3Delete(_ LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, _, key string) error {
	mc, err := newMinio(cfg.Config)
	if err != nil {
		return err
	}
	if err := mc.RemoveObject(ctx, cfgStr(cfg.Config, "bucket"), key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

// s3Test verifies the config end-to-end by checking the bucket exists — the
// cheapest call that exercises endpoint, credentials and bucket.
func s3Test(_ LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile) error {
	mc, err := newMinio(cfg.Config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	bucket := cfgStr(cfg.Config, "bucket")
	ok, err := mc.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("connect to bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", bucket)
	}
	return nil
}

func newMinio(config map[string]any) (*minio.Client, error) {
	endpoint := cfgStr(config, "endpoint")
	bucket := cfgStr(config, "bucket")
	accessKeyID := cfgStr(config, "accessKeyId")
	if endpoint == "" || bucket == "" || accessKeyID == "" {
		return nil, ErrNotConfigured
	}
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, cfgStr(config, "secretAccessKey"), ""),
		Secure: cfgBool(config, "useSsl"),
		Region: cfgStr(config, "region"),
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}
	return mc, nil
}

func cfgBool(config map[string]any, key string) bool {
	b, _ := config[key].(bool)
	return b
}
