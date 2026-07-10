package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
)

type providerConfig struct {
	Directory string `json:"directory" jsonschema:"required,title=存储目录,minLength=1,maxLength=512"`
}

func New() storageint.Registration {
	return storageint.Register(
		"local",
		integration.Presentation{
			Name:        "本地文件存储",
			Description: "把文件保存在服务器本地目录",
			Color:       "#52c41a",
			IconRef:     "antd:HddOutlined",
		},
		config.MustContract[providerConfig](config.Policy{}),
		storageint.Operations[providerConfig]{
			PresignPut: presignPut,
			ResolveURL: resolveURL,
			Delete:     deleteObject,
			Test:       testConnection,
			ObjectPath: objectPath,
		},
	)
}

func presignPut(
	rt storageint.Runtime,
	ctx context.Context,
	_ providerConfig,
	purpose, key, _ string,
	expires time.Duration,
) (string, error) {
	return rt.LocalSignedURL(ctx, "PUT", purpose, key, expires)
}

func resolveURL(
	rt storageint.Runtime,
	ctx context.Context,
	_ providerConfig,
	purpose, key string,
	expires time.Duration,
) (string, error) {
	if rt.VisibilityOf(purpose) == storageint.VisibilityPublic {
		return "/api/files/" + purpose + "/" + key, nil
	}
	return rt.LocalSignedURL(ctx, "GET", purpose, key, expires)
}

func deleteObject(
	_ storageint.Runtime,
	_ context.Context,
	cfg providerConfig,
	_ string,
	key string,
) error {
	path, err := objectPath(cfg, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func testConnection(_ context.Context, _ storageint.Runtime, cfg providerConfig) error {
	if err := os.MkdirAll(cfg.Directory, 0o750); err != nil {
		return fmt.Errorf("create directory %q: %w", cfg.Directory, err)
	}
	probe, err := os.CreateTemp(cfg.Directory, ".probe-*")
	if err != nil {
		return fmt.Errorf("write to directory %q: %w", cfg.Directory, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("write to directory %q: %w", cfg.Directory, err)
	}
	return os.Remove(name)
}
func objectPath(cfg providerConfig, key string) (string, error) {
	path := filepath.Join(cfg.Directory, filepath.FromSlash(key))
	rel, err := filepath.Rel(cfg.Directory, path)
	if err != nil || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("invalid object key %q", key)
	}
	return path, nil
}
