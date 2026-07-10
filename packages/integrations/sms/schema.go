package sms

import "github.com/imbytecat/moonbase/integrations/core/config"

// aliyunSchema and tencentSchema declare each provider's config fields. base
// masks / validates a profile from these flags and the admin form renders from
// them; the driver registry in sms.go pairs each with its send function.
var (
	aliyunSchema = config.Schema{Fields: []config.Field{
		{Key: "accessKeyId", Label: "访问密钥 ID", Type: config.String, Required: true, MaxLen: 128},
		{Key: "accessKeySecret", Label: "访问密钥 Secret", Type: config.String, Secret: true, Required: true, MaxLen: 128},
		{Key: "signName", Label: "短信签名", Type: config.String, Required: true, MaxLen: 64},
		{Key: "templateCode", Label: "模板编号", Type: config.String, Required: true, MaxLen: 64, Help: "模板需包含一个 {code} 变量"},
	}}

	tencentSchema = config.Schema{Fields: []config.Field{
		{Key: "secretId", Label: "密钥 ID", Type: config.String, Required: true, MaxLen: 128},
		{Key: "secretKey", Label: "密钥 Key", Type: config.String, Secret: true, Required: true, MaxLen: 128},
		{Key: "sdkAppId", Label: "SDK 应用 ID", Type: config.String, Required: true, MaxLen: 32},
		{Key: "signName", Label: "短信签名", Type: config.String, Required: true, MaxLen: 64},
		{Key: "templateId", Label: "模板 ID", Type: config.String, Required: true, MaxLen: 32},
		{Key: "region", Label: "区域", Type: config.String, MaxLen: 32, Help: "例如 ap-guangzhou"},
	}}
)
