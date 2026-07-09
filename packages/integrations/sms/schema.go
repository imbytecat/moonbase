package sms

import "github.com/imbytecat/moonbase/packages/integrations/core/schema"

// aliyunSchema and tencentSchema declare each provider's config fields. base
// masks / validates a profile from these flags and the admin form renders from
// them; the driver registry in sms.go pairs each with its send function.
var (
	aliyunSchema = schema.Schema{Fields: []schema.Field{
		{Key: "accessKeyId", Label: "AccessKey ID", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "accessKeySecret", Label: "AccessKey Secret", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
		{Key: "signName", Label: "Sign name", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "templateCode", Label: "Template code", Type: schema.String, Required: true, MaxLen: 64, Help: "Template with a single {code} variable"},
	}}

	tencentSchema = schema.Schema{Fields: []schema.Field{
		{Key: "secretId", Label: "SecretId", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "secretKey", Label: "SecretKey", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
		{Key: "sdkAppId", Label: "SDK AppId", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "signName", Label: "Sign name", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "templateId", Label: "Template id", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "region", Label: "Region", Type: schema.String, MaxLen: 32, Help: "e.g. ap-guangzhou"},
	}}
)
