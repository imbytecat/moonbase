package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	settingsv1 "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

type fakeSettingsQuerier struct {
	repository.Querier
	values  map[string]json.RawMessage
	getFile func(ctx context.Context, id uuid.UUID) (repository.File, error)
	setSite func(ctx context.Context, arg repository.SetSiteWithAssetsParams) error
}

func (f *fakeSettingsQuerier) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	raw, ok := f.values[key]
	if !ok {
		return repository.Setting{}, pgx.ErrNoRows
	}
	return repository.Setting{Key: key, Value: raw}, nil
}

func (f *fakeSettingsQuerier) UpsertSetting(_ context.Context, arg repository.UpsertSettingParams) error {
	f.values[arg.Key] = arg.Value
	return nil
}

func (f *fakeSettingsQuerier) GetFile(ctx context.Context, id uuid.UUID) (repository.File, error) {
	return f.getFile(ctx, id)
}

func (f *fakeSettingsQuerier) SetSiteWithAssets(ctx context.Context, arg repository.SetSiteWithAssetsParams) error {
	if f.setSite != nil {
		return f.setSite(ctx, arg)
	}
	return nil
}

// keyEchoObjectStore resolves any key to a stable URL so read-path tests can
// assert a resolved asset URL without a real backend.
type keyEchoObjectStore struct{}

func (keyEchoObjectStore) PresignPut(context.Context, string, string, string, time.Duration) (string, error) {
	return "", nil
}

func (keyEchoObjectStore) ResolveURL(_ context.Context, _, key string, _ time.Duration) (string, error) {
	return "https://cdn.test/" + key, nil
}

func newSettingsService(q repository.Querier) *SettingsService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSettingsService(settings.NewStore(q), q, noopObjectStore{}, logger)
}

func TestUpdateSettingsPhoneSignupRequiresSmsChannel(t *testing.T) {
	svc := newSettingsService(&fakeSettingsQuerier{values: map[string]json.RawMessage{}})

	_, err := svc.UpdateSettings(t.Context(), connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Auth: &settingsv1.AuthSettings{
			RegistrationEnabled: true,
			SignupIdentifiers:   []string{"username", "phone"},
		},
	}))

	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want failed_precondition (no SMS channel configured)", connect.CodeOf(err))
	}
}

func TestUpdateSettingsEmailSignupRequiresEmailChannel(t *testing.T) {
	svc := newSettingsService(&fakeSettingsQuerier{values: map[string]json.RawMessage{}})

	_, err := svc.UpdateSettings(t.Context(), connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Auth: &settingsv1.AuthSettings{
			RegistrationEnabled: true,
			SignupIdentifiers:   []string{"email"},
		},
	}))

	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want failed_precondition (no email channel configured)", connect.CodeOf(err))
	}
}

func TestUpdateSettingsUsernameSignupNeedsNoChannel(t *testing.T) {
	svc := newSettingsService(&fakeSettingsQuerier{values: map[string]json.RawMessage{}})

	resp, err := svc.UpdateSettings(t.Context(), connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Auth: &settingsv1.AuthSettings{
			RegistrationEnabled: true,
			SignupIdentifiers:   []string{"username"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	got := resp.Msg.GetAuth().GetSignupIdentifiers()
	if len(got) != 1 || got[0] != "username" {
		t.Fatalf("signup identifiers = %v, want [username]", got)
	}
}

// GetSiteInfo resolves the public asset URLs by looking the brand file ids up
// in the ledger and turning each object key into a URL (ADR-0003 read side).
func TestGetSiteInfoResolvesAssetURLsFromFileIDs(t *testing.T) {
	logoID := uuid.New()
	faviconID := uuid.New()
	site, err := json.Marshal(settings.Site{
		Name:          "Acme",
		LogoFileID:    logoID.String(),
		FaviconFileID: faviconID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	q := &fakeSettingsQuerier{
		values: map[string]json.RawMessage{"site": site},
		getFile: func(_ context.Context, id uuid.UUID) (repository.File, error) {
			switch id {
			case logoID:
				return repository.File{ID: id, ObjectKey: "site/logo-a.png", Purpose: "site-assets"}, nil
			case faviconID:
				return repository.File{ID: id, ObjectKey: "site/favicon-b.ico", Purpose: "site-assets"}, nil
			}
			return repository.File{}, pgx.ErrNoRows
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewSettingsService(settings.NewStore(q), q, keyEchoObjectStore{}, logger)

	resp, err := svc.GetSiteInfo(t.Context(), connect.NewRequest(&settingsv1.GetSiteInfoRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Msg.GetLogoUrl(); got != "https://cdn.test/site/logo-a.png" {
		t.Fatalf("logo_url = %q, want URL resolved from the logo file's object key", got)
	}
	if got := resp.Msg.GetFaviconUrl(); got != "https://cdn.test/site/favicon-b.ico" {
		t.Fatalf("favicon_url = %q, want URL resolved from the favicon file's object key", got)
	}
}

// A brand asset can only reference a file uploaded as a site asset. A file of a
// different purpose is rejected and the settings are never saved.
func TestUpdateSettingsRejectsNonSiteAssetFile(t *testing.T) {
	saved := false
	q := &fakeSettingsQuerier{
		values: map[string]json.RawMessage{},
		getFile: func(_ context.Context, id uuid.UUID) (repository.File, error) {
			return repository.File{ID: id, ObjectKey: "avatars/x.png", Purpose: "avatars"}, nil
		},
		setSite: func(context.Context, repository.SetSiteWithAssetsParams) error {
			saved = true
			return nil
		},
	}
	svc := newSettingsService(q)

	_, err := svc.UpdateSettings(t.Context(), connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Site: &settingsv1.SiteSettings{
			Name:       "Acme",
			LogoFileId: uuid.NewString(),
		},
	}))

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument for a file that isn't a site asset", connect.CodeOf(err))
	}
	if saved {
		t.Fatal("must not save site settings pointing at a non-asset file")
	}
}

func TestUpdateSettingsEmptyIdentifiersDefaultsToUsername(t *testing.T) {
	svc := newSettingsService(&fakeSettingsQuerier{values: map[string]json.RawMessage{}})

	resp, err := svc.UpdateSettings(t.Context(), connect.NewRequest(&settingsv1.UpdateSettingsRequest{
		Auth: &settingsv1.AuthSettings{RegistrationEnabled: true},
	}))
	if err != nil {
		t.Fatal(err)
	}

	got := resp.Msg.GetAuth().GetSignupIdentifiers()
	if len(got) != 1 || got[0] != "username" {
		t.Fatalf("signup identifiers = %v, want [username]", got)
	}
}
