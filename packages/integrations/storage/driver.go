package storage

import (
	"context"
	"errors"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = errors.New("file storage is not configured")

type Ops struct {
	PresignPut func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key, contentType string, expires time.Duration) (string, error)
	ResolveURL func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key string, expires time.Duration) (string, error)
	Delete     func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile, purpose, key string) error
	Test       func(rt LocalRuntime, ctx context.Context, cfg kitsettings.GenericProfile) error
}

var Registry = integration.MustRegistry([]integration.Entry[Ops]{
	{
		Key:          "s3",
		Presentation: integration.Presentation{Name: "S3 兼容存储", Description: "连接兼容 S3 协议的对象存储服务", Color: "#1677ff", IconRef: "antd:CloudServerOutlined"},
		Config:       s3Schema,
		Ops: Ops{
			PresignPut: s3PresignPut,
			ResolveURL: s3ResolveURL,
			Delete:     s3Delete,
			Test:       s3Test,
		},
	},
	{
		Key:          "local",
		Presentation: integration.Presentation{Name: "本地文件存储", Description: "把文件保存在服务器本地目录", Color: "#52c41a", IconRef: "antd:HddOutlined"},
		Config:       localSchema,
		Ops: Ops{
			PresignPut: localPresignPut,
			ResolveURL: localResolveURL,
			Delete:     localDelete,
			Test:       localTest,
		},
	},
})

func Providers() []string { return Registry.Names() }

func DriverFor(provider string) (integration.Entry[Ops], bool) { return Registry.EntryFor(provider) }

func ProfileUsable(p kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(p.Provider, p.Config)
}
