package storage

import "github.com/imbytecat/moonbase/server/integrationkit/schema"

var (
	localSchema = schema.Schema{Fields: []schema.Field{
		{Key: "directory", Label: "Directory", Type: schema.String, MaxLen: 512},
	}}

	s3Schema = schema.Schema{Fields: []schema.Field{
		{Key: "endpoint", Label: "Endpoint", Type: schema.String, Required: true, MaxLen: 253},
		{Key: "region", Label: "Region", Type: schema.String, MaxLen: 64},
		{Key: "bucket", Label: "Bucket", Type: schema.String, Required: true, MaxLen: 63},
		{Key: "accessKeyId", Label: "Access key ID", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "secretAccessKey", Label: "Secret access key", Type: schema.String, Secret: true, MaxLen: 128},
		{Key: "useSsl", Label: "Use SSL", Type: schema.Bool},
		{Key: "publicBaseUrl", Label: "Public base URL", Type: schema.String, MaxLen: 512},
	}}
)
