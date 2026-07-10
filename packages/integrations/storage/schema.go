package storage

import "github.com/imbytecat/moonbase/integrations/core/config"

var (
	localSchema = config.Schema{Fields: []config.Field{
		{Key: "directory", Label: "存储目录", Type: config.String, MaxLen: 512},
	}}

	s3Schema = config.Schema{Fields: []config.Field{
		{Key: "endpoint", Label: "服务地址", Type: config.String, Required: true, MaxLen: 253},
		{Key: "region", Label: "区域", Type: config.String, MaxLen: 64},
		{Key: "bucket", Label: "存储桶", Type: config.String, Required: true, MaxLen: 63},
		{Key: "accessKeyId", Label: "访问密钥 ID", Type: config.String, Required: true, MaxLen: 128},
		{Key: "secretAccessKey", Label: "访问密钥 Secret", Type: config.String, Secret: true, MaxLen: 128},
		{Key: "useSsl", Label: "使用 SSL", Type: config.Bool},
		{Key: "publicBaseUrl", Label: "公开访问地址", Type: config.String, MaxLen: 512},
	}}
)
