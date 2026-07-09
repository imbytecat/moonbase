package storage

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	localSchema = schema.Schema{Fields: []schema.Field{
		{Key: "directory", Label: "存储目录", Type: schema.String, MaxLen: 512},
	}}

	s3Schema = schema.Schema{Fields: []schema.Field{
		{Key: "endpoint", Label: "服务地址", Type: schema.String, Required: true, MaxLen: 253},
		{Key: "region", Label: "区域", Type: schema.String, MaxLen: 64},
		{Key: "bucket", Label: "存储桶", Type: schema.String, Required: true, MaxLen: 63},
		{Key: "accessKeyId", Label: "访问密钥 ID", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "secretAccessKey", Label: "访问密钥 Secret", Type: schema.String, Secret: true, MaxLen: 128},
		{Key: "useSsl", Label: "使用 SSL", Type: schema.Bool},
		{Key: "publicBaseUrl", Label: "公开访问地址", Type: schema.String, MaxLen: 512},
	}}
)
