package wechat

import (
	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
)

type providerConfig struct {
	Key       string `json:"key"       jsonschema:"required,title=标识,minLength=2,maxLength=32,pattern=^[a-z][a-z0-9-]+$"`
	AppID     string `json:"appId"     jsonschema:"required,title=应用 ID,minLength=1,maxLength=64"`
	AppSecret string `json:"appSecret" jsonschema:"required,title=应用 Secret,minLength=1,maxLength=128"`
}

func New() oauthint.Registration {
	return oauthint.Register(
		"wechat",
		integration.Presentation{
			Name:        "微信开放平台",
			Description: "使用网站应用扫码登录",
			Color:       "#07c160",
			IconRef:     "antd:WechatOutlined",
		},
		config.MustContract[providerConfig](
			config.Policy{Secrets: []string{"/appSecret"}, CreateOnly: []string{"/key"}},
		),
		oauthint.Operations[providerConfig]{
			AuthorizeURL: wechatAuthorizeURL,
			Exchange:     wechatExchange,
		},
	)
}
