package rpc

import (
	"encoding/json"
	"testing"

	"connectrpc.com/connect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
)

func TestSmsProfileUsesTypedContractAndWriteOnlySecrets(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newSystemService(q)
	created, err := svc.CreateSmsProfile(t.Context(), connect.NewRequest(&systemv1.CreateSmsProfileRequest{Profile: &systemv1.ProfileInput{
		Name: "腾讯云", Provider: "tencent", Config: profileWrite(t, map[string]any{
			"secretId": "id", "sdkAppId": "app", "signName": "签名", "templateId": "tpl", "region": "ap-guangzhou",
		}, map[string]string{"/secretKey": "secret"}),
	}}))
	if err != nil {
		t.Fatal(err)
	}
	profile := created.Msg.GetProfile()
	if !profile.GetConfigValid() || !secretSet(profile, "/secretKey") {
		t.Fatalf("profile = %+v", profile)
	}
	if _, exists := configMap(profile)["secretKey"]; exists {
		t.Fatal("secret must not appear in values")
	}

	_, err = svc.UpdateSmsProfile(t.Context(), connect.NewRequest(&systemv1.UpdateSmsProfileRequest{Profile: &systemv1.ProfileInput{
		Id: profile.GetId(), Name: "腾讯云更新", Provider: "tencent", Config: profileWrite(t, map[string]any{
			"secretId": "id-2", "sdkAppId": "app", "signName": "签名", "templateId": "tpl", "region": "ap-shanghai",
		}, nil),
	}}))
	if err != nil {
		t.Fatal(err)
	}
	var stored settings.Sms
	if err := json.Unmarshal(q.rows["sms"], &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Profiles[0].Config["secretKey"]; got != "secret" {
		t.Fatalf("secret = %v, want retained", got)
	}
}

func TestTencentSmsRegionMustBePersistedExplicitly(t *testing.T) {
	svc, _ := newSystemService(newMemSettingsQuerier())
	_, err := svc.CreateSmsProfile(t.Context(), connect.NewRequest(&systemv1.CreateSmsProfileRequest{Profile: &systemv1.ProfileInput{
		Name: "腾讯云", Provider: "tencent", Config: profileWrite(t, map[string]any{
			"secretId": "id", "sdkAppId": "app", "signName": "签名", "templateId": "tpl",
		}, map[string]string{"/secretKey": "secret"}),
	}}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestDescribeSmsProvidersReturnsOrderedSelfDescriptions(t *testing.T) {
	svc, _ := newSystemService(newMemSettingsQuerier())
	resp, err := svc.DescribeSmsProviders(t.Context(), connect.NewRequest(&systemv1.DescribeSmsProvidersRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Msg.GetPurposes(); len(got) != 1 || got[0].GetKey() != sms.PurposeVerification {
		t.Fatalf("purposes = %+v", got)
	}
	providers := resp.Msg.GetProviders()
	if len(providers) != 2 || providers[0].GetKey() != "aliyun" || providers[1].GetKey() != "tencent" {
		t.Fatalf("providers = %+v", providers)
	}
}
