package rpc_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	settingsv1 "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/settings/v1/settingsv1connect"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// End to end: replacing the site logo moves the 'site'/'logo' attachment to the
// new file and leaves the old one unattached for the sweep, while GetSiteInfo
// keeps resolving a URL from whichever file is current (ADR-0003).
func TestReplaceSiteLogoTransfersAttachment(t *testing.T) {
	baseURL, client, pool := newStackWithPool(t)
	ctx := t.Context()

	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []systemcodec.StorageProfile{{
			Id:       "local-site",
			Name:     "Local Site",
			Provider: "local",
			Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
		}},
		Bindings: map[string][]string{storage.PurposeSiteAssets: {"local-site"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)
	settingsClient := settingsv1connect.NewSettingsServiceClient(client, baseURL)
	storageClient := storagev1connect.NewStorageServiceClient(client, baseURL)

	presignLogo := func() string {
		t.Helper()
		resp, err := storageClient.PresignSiteAssetUpload(ctx, connect.NewRequest(&storagev1.PresignSiteAssetUploadRequest{
			Kind:          "logo",
			ContentType:   "image/png",
			ContentLength: 1024,
		}))
		if err != nil {
			t.Fatal(err)
		}
		return resp.Msg.GetFileId()
	}
	attachmentCount := func(fileID string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM file_attachments WHERE file_id = $1`, fileID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	saveLogo := func(fileID string) {
		t.Helper()
		if _, err := settingsClient.UpdateSettings(ctx, connect.NewRequest(&settingsv1.UpdateSettingsRequest{
			Site: &settingsv1.SiteSettings{Name: "Acme", LogoFileId: fileID},
		})); err != nil {
			t.Fatal(err)
		}
	}

	fileA := presignLogo()
	fileB := presignLogo()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM settings WHERE key = 'site'`)
		_, _ = pool.Exec(bg, `DELETE FROM file_attachments WHERE file_id = ANY($1)`, []string{fileA, fileB})
		_, _ = pool.Exec(bg, `DELETE FROM files WHERE id = ANY($1)`, []string{fileA, fileB})
	})

	saveLogo(fileA)
	if got := attachmentCount(fileA); got != 1 {
		t.Fatalf("after first save, logo file A attachments = %d, want 1", got)
	}
	first, err := settingsClient.GetSiteInfo(ctx, connect.NewRequest(&settingsv1.GetSiteInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if first.Msg.GetLogoUrl() == "" {
		t.Fatal("GetSiteInfo must resolve logo_url after saving a logo")
	}

	saveLogo(fileB)
	if got := attachmentCount(fileB); got != 1 {
		t.Fatalf("after replacement, logo file B attachments = %d, want 1", got)
	}
	if got := attachmentCount(fileA); got != 0 {
		t.Fatalf("after replacement, old logo file A attachments = %d, want 0 (unattached)", got)
	}
	second, err := settingsClient.GetSiteInfo(ctx, connect.NewRequest(&settingsv1.GetSiteInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if second.Msg.GetLogoUrl() == "" {
		t.Fatal("GetSiteInfo must resolve the new logo_url after replacement")
	}
}

// The startup backfill mints a file + attachment for a legacy raw logoKey and
// rewrites the setting to reference it by id; a second run is a no-op (ADR-0003
// 存量回填, idempotent).
func TestBackfillSiteAssetsIsIdempotent(t *testing.T) {
	_, _, pool := newStackWithPool(t)
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const logoKey = "site/logo-legacy.png"
	legacy, err := json.Marshal(map[string]string{"name": "Legacy Co", "logoKey": logoKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.New(pool).UpsertSetting(ctx, repository.UpsertSettingParams{Key: "site", Value: legacy}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM settings WHERE key = 'site'`)
		_, _ = pool.Exec(bg, `DELETE FROM file_attachments WHERE owner_type = 'site'`)
		_, _ = pool.Exec(bg, `DELETE FROM files WHERE object_key = $1`, logoKey)
	})

	if err := settings.BackfillSiteAssets(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}

	countFiles := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM files WHERE object_key = $1`, logoKey).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := countFiles(); got != 1 {
		t.Fatalf("files minted for legacy logo key = %d, want 1", got)
	}

	var (
		fileID  uuid.UUID
		purpose string
	)
	if err := pool.QueryRow(ctx,
		`SELECT id, purpose FROM files WHERE object_key = $1`, logoKey).Scan(&fileID, &purpose); err != nil {
		t.Fatal(err)
	}
	if purpose != "site-assets" {
		t.Fatalf("minted purpose = %q, want site-assets", purpose)
	}

	var attachments int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM file_attachments WHERE file_id = $1 AND owner_type = 'site' AND owner_id = 'logo'`,
		fileID).Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments != 1 {
		t.Fatalf("logo attachment count = %d, want 1", attachments)
	}

	var stored struct {
		LogoKey    string `json:"logoKey"`
		LogoFileID string `json:"logoFileId"`
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'site'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.LogoFileID != fileID.String() {
		t.Fatalf("setting logoFileId = %q, want minted file %s", stored.LogoFileID, fileID)
	}
	if stored.LogoKey != "" {
		t.Fatalf("setting still holds raw logoKey %q, want it dropped", stored.LogoKey)
	}

	if err := settings.BackfillSiteAssets(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}
	if got := countFiles(); got != 1 {
		t.Fatalf("after second backfill, files for legacy key = %d, want still 1 (idempotent)", got)
	}
}
