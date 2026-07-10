package oidc

import (
	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
)

type providerConfig struct {
	Key          string `json:"key" jsonschema:"required,title=标识,minLength=2,maxLength=32,pattern=^[a-z][a-z0-9-]+$"`
	Issuer       string `json:"issuer" jsonschema:"required,title=签发方地址,minLength=1,maxLength=512,format=uri"`
	ClientID     string `json:"clientId" jsonschema:"required,title=客户端 ID,minLength=1,maxLength=256"`
	ClientSecret string `json:"clientSecret" jsonschema:"required,title=客户端 Secret,minLength=1,maxLength=256"`
	Scopes       string `json:"scopes,omitempty" jsonschema:"title=授权范围,maxLength=256"`
}

func New() oauthint.Registration {
	return oauthint.Register(
		"oidc",
		integration.Presentation{Name: "OpenID Connect", Description: "连接支持发现、ID Token 校验与 PKCE 的身份提供方", Color: "#1677ff", IconRef: "antd:IdcardOutlined"},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{"/clientSecret"}, CreateOnly: []string{"/key"}}),
		oauthint.Operations[providerConfig]{AuthorizeURL: oidcAuthorizeURL, Exchange: oidcExchange},
	)
}
