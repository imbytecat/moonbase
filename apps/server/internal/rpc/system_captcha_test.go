package rpc

import (
	"encoding/json"
	"testing"

	"connectrpc.com/connect"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func TestCaptchaProfileUsesTypedContractAndWriteOnlySecret(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newSystemService(q)
	created, err := svc.CreateCaptchaProfile(
		t.Context(),
		connect.NewRequest(
			&systemv1.CreateCaptchaProfileRequest{
				Profile: &systemv1.ProfileInput{
					Name:     "Turnstile",
					Provider: "turnstile",
					Config: profileWrite(
						t,
						map[string]any{"siteKey": "site"},
						map[string]string{"/secretKey": "secret"},
					),
				},
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	p := created.Msg.GetProfile()
	if !p.GetConfigValid() || !secretSet(p, "/secretKey") {
		t.Fatalf("profile=%+v", p)
	}
	if _, ok := configMap(p)["secretKey"]; ok {
		t.Fatal("secret leaked")
	}
	_, err = svc.UpdateCaptchaProfile(
		t.Context(),
		connect.NewRequest(
			&systemv1.UpdateCaptchaProfileRequest{
				Profile: &systemv1.ProfileInput{
					Id:       p.GetId(),
					Name:     "更新",
					Provider: "turnstile",
					Config:   profileWrite(t, map[string]any{"siteKey": "site2"}, nil),
				},
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	var stored settings.Captcha
	if err := json.Unmarshal(q.rows["captcha"], &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Profiles[0].Config["secretKey"] != "secret" {
		t.Fatal("secret not retained")
	}
}
