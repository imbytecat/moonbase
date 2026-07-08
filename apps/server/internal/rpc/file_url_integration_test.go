package rpc_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	settingsv1 "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/settings/v1/settingsv1connect"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// End to end over the real wire (ADR-0004): GetSiteInfo hands out the
// permanent /f/{file_id} URL, and an anonymous client (no session — the login
// page scenario) fetches the actual bytes through it with the year-long
// immutable cache header. Unknown ids are 404.
func TestPermanentFileURLServesSiteLogoAnonymously(t *testing.T) {
	baseURL, client, pool := newStackWithPool(t)
	serverURL := strings.TrimSuffix(baseURL, "/api")
	ctx := t.Context()

	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []systemcodec.StorageProfile{{
			Id:       "local-f",
			Name:     "Local F",
			Provider: "local",
			Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
		}},
		Bindings: map[string][]string{storage.PurposeSiteAssets: {"local-f"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)
	settingsClient := settingsv1connect.NewSettingsServiceClient(client, baseURL)
	storageClient := storagev1connect.NewStorageServiceClient(client, baseURL)

	presigned, err := storageClient.PresignSiteAssetUpload(ctx, connect.NewRequest(&storagev1.PresignSiteAssetUploadRequest{
		Kind:          "logo",
		ContentType:   "image/png",
		ContentLength: 4,
	}))
	if err != nil {
		t.Fatal(err)
	}
	fileID := presigned.Msg.GetFileId()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM settings WHERE key = 'site'`)
		_, _ = pool.Exec(bg, `DELETE FROM file_attachments WHERE file_id = $1`, fileID)
		_, _ = pool.Exec(bg, `DELETE FROM files WHERE id = $1`, fileID)
	})

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		serverURL+presigned.Msg.GetUploadUrl(), strings.NewReader("logo"))
	if err != nil {
		t.Fatal(err)
	}
	putRes, err := client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("PUT upload status = %d, want 200", putRes.StatusCode)
	}

	if _, err := settingsClient.UpdateSettings(ctx, connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Site: &settingsv1.SiteSettings{Name: "Acme", LogoFileId: fileID},
	})); err != nil {
		t.Fatal(err)
	}

	info, err := settingsClient.GetSiteInfo(ctx, connect.NewRequest(&settingsv1.GetSiteInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Msg.GetLogoUrl(), "/f/"+fileID; got != want {
		t.Fatalf("logo_url = %q, want %q", got, want)
	}

	anon := &http.Client{}
	res, err := anon.Get(serverURL + info.Msg.GetLogoUrl())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "logo" {
		t.Fatalf("anonymous GET /f = %d %q, want 200 \"logo\"", res.StatusCode, body)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want public immutable year", got)
	}

	missing, err := anon.Get(serverURL + "/f/" + uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	_ = missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown /f = %d, want 404", missing.StatusCode)
	}
}
