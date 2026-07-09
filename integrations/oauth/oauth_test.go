package oauth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
)

func TestClientAuthorizeURLUsesBoundUsableProfile(t *testing.T) {
	client := NewClient(func(context.Context) (Config, error) {
		return Config{
			Profiles: []systemcodec.OauthProfile{{
				Id:       "wechat-login",
				Key:      "wechat",
				Name:     "WeChat",
				Provider: "wechat",
				Wechat: systemcodec.WechatOauthConfig{
					AppId:     "app-id",
					AppSecret: "app-secret",
				},
			}},
			Bindings: map[string][]string{PurposeLogin: {"wechat-login"}},
		}, nil
	})

	url, secrets, err := client.AuthorizeURL(t.Context(), "wechat", "https://app.example.com/callback", "state-token")
	if err != nil {
		t.Fatal(err)
	}
	if secrets != (FlowSecrets{}) {
		t.Fatalf("secrets = %+v, want empty WeChat secrets", secrets)
	}
	for _, want := range []string{
		"https://open.weixin.qq.com/connect/qrconnect?",
		"appid=app-id",
		"redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback",
		"state=state-token",
		"#wechat_redirect",
	} {
		if !strings.Contains(url, want) {
			t.Fatalf("authorize URL %q missing %q", url, want)
		}
	}
}

func TestClientAuthorizeURLRejectsUnboundProfile(t *testing.T) {
	client := NewClient(func(context.Context) (Config, error) {
		return Config{
			Profiles: []systemcodec.OauthProfile{{
				Id:       "wechat-login",
				Key:      "wechat",
				Provider: "wechat",
				Wechat: systemcodec.WechatOauthConfig{
					AppId:     "app-id",
					AppSecret: "app-secret",
				},
			}},
			Bindings: map[string][]string{},
		}, nil
	})

	_, _, err := client.AuthorizeURL(t.Context(), "wechat", "https://app.example.com/callback", "state-token")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
