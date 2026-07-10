package s3

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
)

type providerConfig struct {
	Endpoint        string `json:"endpoint" jsonschema:"required,title=服务地址,minLength=1,maxLength=253"`
	Region          string `json:"region,omitempty" jsonschema:"title=区域,maxLength=64"`
	Bucket          string `json:"bucket" jsonschema:"required,title=存储桶,minLength=1,maxLength=63"`
	AccessKeyID     string `json:"accessKeyId" jsonschema:"required,title=访问密钥 ID,minLength=1,maxLength=128"`
	SecretAccessKey string `json:"secretAccessKey" jsonschema:"required,title=访问密钥 Secret,minLength=1,maxLength=128"`
	UseSSL          bool   `json:"useSsl" jsonschema:"title=使用 SSL"`
	PublicBaseURL   string `json:"publicBaseUrl,omitempty" jsonschema:"title=公开访问地址,maxLength=512"`
}

func New() storageint.Registration {
	return storageint.Register("s3", integration.Presentation{Name: "S3 兼容存储", Description: "连接兼容 S3 协议的对象存储服务", Color: "#1677ff", IconRef: "antd:CloudServerOutlined"}, config.MustContract[providerConfig](config.Policy{Secrets: []string{"/secretAccessKey"}}), storageint.Operations[providerConfig]{PresignPut: presignPut, ResolveURL: resolveURL, Delete: deleteObject, Test: testConnection})
}
func client(cfg providerConfig) (*minio.Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""), Secure: cfg.UseSSL, Region: cfg.Region})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}
	return mc, nil
}
func presignPut(_ storageint.Runtime, ctx context.Context, cfg providerConfig, _ string, key, contentType string, expires time.Duration) (string, error) {
	mc, err := client(cfg)
	if err != nil {
		return "", err
	}
	u, err := mc.Presign(ctx, "PUT", cfg.Bucket, key, expires, url.Values{"Content-Type": {contentType}})
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return u.String(), nil
}
func resolveURL(_ storageint.Runtime, ctx context.Context, cfg providerConfig, _ string, key string, expires time.Duration) (string, error) {
	if cfg.PublicBaseURL != "" {
		return strings.TrimSuffix(cfg.PublicBaseURL, "/") + "/" + strings.TrimPrefix(key, "/"), nil
	}
	mc, err := client(cfg)
	if err != nil {
		return "", err
	}
	u, err := mc.PresignedGetObject(ctx, cfg.Bucket, key, expires, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return u.String(), nil
}
func deleteObject(_ storageint.Runtime, ctx context.Context, cfg providerConfig, _ string, key string) error {
	mc, err := client(cfg)
	if err != nil {
		return err
	}
	if err := mc.RemoveObject(ctx, cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}
func testConnection(ctx context.Context, _ storageint.Runtime, cfg providerConfig) error {
	mc, err := client(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ok, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("connect to bucket: %w", err)
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}
	return nil
}
