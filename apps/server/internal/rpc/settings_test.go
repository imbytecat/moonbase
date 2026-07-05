package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	settingsv1 "github.com/imbytecat/moonbase/server/internal/gen/settings/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

type fakeSettingsQuerier struct {
	repository.Querier
	values map[string]json.RawMessage
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

func newSettingsService(q repository.Querier) *SettingsService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSettingsService(settings.NewStore(q), noopObjectStore{}, logger)
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
