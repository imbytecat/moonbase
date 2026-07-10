package cloudflare

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/integrations/email"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestRegistrationSendsWithPrivateTypedConfig(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.cloudflare.com/client/v4/accounts/account-1/email/sending/send" {
			t.Fatalf("URL = %q", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(request.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["from"] != "Moonbase <noreply@example.com>" {
			t.Fatalf("from = %q", payload["from"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
		}, nil
	})}
	registry := email.MustRegistry(New(client))

	err := registry.Send(t.Context(), settings.GenericProfile{
		Provider: "cloudflare",
		Config: map[string]any{
			"fromAddress": "noreply@example.com",
			"fromName":    "Moonbase",
			"accountId":   "account-1",
			"apiToken":    "token-1",
		},
	}, email.Message{To: "user@example.com", Subject: "主题", TextBody: "正文"})
	if err != nil {
		t.Fatal(err)
	}
}
