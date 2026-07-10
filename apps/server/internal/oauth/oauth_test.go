package oauth

import (
	"context"
	"errors"
	"strings"
	"testing"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

func oauthSettings(bound bool) Config {
	settings := Config{Profiles: []kitsettings.GenericProfile{{
		Id: "wechat-login", Name: "微信", Provider: "wechat",
		Config: map[string]any{"key": "wechat", "appId": "app-id", "appSecret": "app-secret"},
	}}, Bindings: map[string][]string{}}
	if bound {
		settings.Bindings[PurposeLogin] = []string{"wechat-login"}
	}
	return settings
}

func TestClientUsesOnlyBoundUsableProfile(t *testing.T) {
	client := NewClient(func(context.Context) (Config, error) { return oauthSettings(true), nil }, NewRegistry())
	url, secrets, err := client.AuthorizeURL(t.Context(), "wechat", "https://app.example.com/callback", "state-token")
	if err != nil {
		t.Fatal(err)
	}
	if secrets != (FlowSecrets{}) || !strings.Contains(url, "appid=app-id") || !strings.Contains(url, "state=state-token") {
		t.Fatalf("url=%q secrets=%+v", url, secrets)
	}

	unbound := NewClient(func(context.Context) (Config, error) { return oauthSettings(false), nil }, NewRegistry())
	if _, _, err := unbound.AuthorizeURL(t.Context(), "wechat", "https://app.example.com/callback", "state"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err=%v, want ErrNotConfigured", err)
	}
}

func TestProviderOptionsExcludeInvalidProfiles(t *testing.T) {
	settings := oauthSettings(true)
	delete(settings.Profiles[0].Config, "appSecret")
	client := NewClient(func(context.Context) (Config, error) { return settings, nil }, NewRegistry())
	options, err := client.ProviderOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 0 {
		t.Fatalf("options=%+v", options)
	}
}
