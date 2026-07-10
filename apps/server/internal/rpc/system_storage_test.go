package rpc

import (
	"testing"

	"connectrpc.com/connect"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func TestLocalStorageDirectoryMustBePersistedExplicitly(t *testing.T) {
	svc, _ := newSystemService(newMemSettingsQuerier())
	_, err := svc.CreateStorageProfile(t.Context(), connect.NewRequest(&systemv1.CreateStorageProfileRequest{Profile: &systemv1.ProfileInput{Name: "本地", Provider: "local", Config: profileWrite(t, map[string]any{}, nil)}}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestInvalidStoredStorageProfileHasSafeProjection(t *testing.T) {
	q := newMemSettingsQuerier()
	store := settings.NewStore(q)
	if err := store.SetStorage(t.Context(), settings.Storage{Profiles: []kitsettings.GenericProfile{{Id: "bad", Name: "坏配置", Provider: "s3", Config: map[string]any{"endpoint": "s3.example.com", "secretAccessKey": "must-not-leak"}}}}); err != nil {
		t.Fatal(err)
	}
	svc, _ := newSystemService(q)
	resp, err := svc.GetSystemSettings(t.Context(), connect.NewRequest(&systemv1.GetSystemSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	profile := resp.Msg.GetStorage().GetProfiles()[0]
	if profile.GetConfigValid() {
		t.Fatal("invalid stored config must be marked invalid")
	}
	if _, exists := configMap(profile)["secretAccessKey"]; exists {
		t.Fatal("invalid stored secret must not leak")
	}
	if !secretSet(profile, "/secretAccessKey") {
		t.Fatal("stored secret state should remain visible")
	}
}
