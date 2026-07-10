package rpc

import (
	"testing"

	"connectrpc.com/connect"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func TestInvalidStoredLlmProfileHasSafeProjection(t *testing.T) {
	q := newMemSettingsQuerier()
	store := settings.NewStore(q)
	if err := store.SetLlm(t.Context(), settings.Llm{Profiles: []kitsettings.GenericProfile{{Id: "bad", Provider: "openai", Config: map[string]any{"apiKey": "secret"}}}}); err != nil {
		t.Fatal(err)
	}
	svc, _ := newLlmSystemService(q)
	resp, err := svc.GetSystemSettings(t.Context(), connect.NewRequest(&systemv1.GetSystemSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	p := resp.Msg.GetLlm().GetProfiles()[0]
	if p.GetConfigValid() {
		t.Fatal("bad profile marked valid")
	}
	if _, ok := configMap(p)["apiKey"]; ok {
		t.Fatal("secret leaked")
	}
	if !secretSet(p, "/apiKey") {
		t.Fatal("secret state missing")
	}
}
