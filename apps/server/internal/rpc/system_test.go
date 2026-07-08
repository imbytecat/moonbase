package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	"github.com/imbytecat/moonbase/server/integrations/llm"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	stg "github.com/imbytecat/moonbase/server/internal/storage"
)

// memSettingsQuerier is an in-memory settings table: enough Querier surface
// for the settings.Store used by SystemService.
type memSettingsQuerier struct {
	repository.Querier
	rows map[string][]byte
}

func newMemSettingsQuerier() *memSettingsQuerier {
	return &memSettingsQuerier{rows: map[string][]byte{}}
}

func (m *memSettingsQuerier) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	raw, ok := m.rows[key]
	if !ok {
		return repository.Setting{}, pgx.ErrNoRows
	}
	return repository.Setting{Key: key, Value: raw}, nil
}

func (m *memSettingsQuerier) UpsertSetting(_ context.Context, arg repository.UpsertSettingParams) error {
	m.rows[arg.Key] = arg.Value
	return nil
}

type okTester struct{ lastTested systemcodec.StorageProfile }

func (t *okTester) TestConnection(_ context.Context, cfg systemcodec.StorageProfile) error {
	t.lastTested = cfg
	return nil
}

func newSystemService(q repository.Querier) (*SystemService, *okTester) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tester := &okTester{}
	return NewSystemService(settings.NewStore(q), nil, tester, nil, nil, nil, logger), tester
}

func TestStorageProfileCRUDAndBinding(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateStorageProfile(ctx, connect.NewRequest(&systemv1.CreateStorageProfileRequest{
		Profile: &systemv1.StorageProfile{
			Name:     "public assets",
			Provider: "s3",
			S3: &systemv1.S3StorageConfig{
				Endpoint:        "s3.example.com",
				Bucket:          "assets",
				AccessKeyId:     "AK",
				SecretAccessKey: "SECRET",
				PublicBaseUrl:   "https://cdn.example.com",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	profile := created.Msg.GetProfile()
	if profile.GetId() == "" {
		t.Fatal("create must assign an id")
	}
	if profile.GetS3().GetSecretAccessKey() != "" {
		t.Fatal("secret must never be echoed back")
	}
	if !profile.GetS3().GetSecretAccessKeySet() {
		t.Fatal("secret_set must report a stored secret")
	}

	if _, err := svc.BindStoragePurpose(ctx, connect.NewRequest(&systemv1.BindStoragePurposeRequest{
		Purpose:   stg.PurposeSiteAssets,
		ProfileId: profile.GetId(),
	})); err != nil {
		t.Fatal(err)
	}

	_, err = svc.BindStoragePurpose(ctx, connect.NewRequest(&systemv1.BindStoragePurposeRequest{
		Purpose:   "not-a-purpose",
		ProfileId: profile.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown purpose: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	_, err = svc.DeleteStorageProfile(ctx, connect.NewRequest(&systemv1.DeleteStorageProfileRequest{
		Id: profile.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("delete bound profile: code = %v, want failed_precondition", connect.CodeOf(err))
	}

	if _, err := svc.BindStoragePurpose(ctx, connect.NewRequest(&systemv1.BindStoragePurposeRequest{
		Purpose:   stg.PurposeSiteAssets,
		ProfileId: "",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteStorageProfile(ctx, connect.NewRequest(&systemv1.DeleteStorageProfileRequest{
		Id: profile.GetId(),
	})); err != nil {
		t.Fatalf("delete after unbind: %v", err)
	}

	var stored settings.Storage
	if err := json.Unmarshal(q.rows["storage"], &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Profiles) != 0 {
		t.Fatalf("profiles after delete = %d, want 0", len(stored.Profiles))
	}
}

func TestUpdateStorageProfileKeepsSecretWhenEmpty(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateStorageProfile(ctx, connect.NewRequest(&systemv1.CreateStorageProfileRequest{
		Profile: &systemv1.StorageProfile{
			Name:     "private",
			Provider: "s3",
			S3: &systemv1.S3StorageConfig{
				Endpoint:        "s3.example.com",
				Bucket:          "files",
				AccessKeyId:     "AK",
				SecretAccessKey: "ORIGINAL",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.GetProfile().GetId()

	if _, err := svc.UpdateStorageProfile(ctx, connect.NewRequest(&systemv1.UpdateStorageProfileRequest{
		Profile: &systemv1.StorageProfile{
			Id:       id,
			Name:     "private (renamed)",
			Provider: "s3",
			S3: &systemv1.S3StorageConfig{
				Endpoint:    "s3.example.com",
				Bucket:      "files",
				AccessKeyId: "AK2",
			},
		},
	})); err != nil {
		t.Fatal(err)
	}

	var stored settings.Storage
	if err := json.Unmarshal(q.rows["storage"], &stored); err != nil {
		t.Fatal(err)
	}
	p, ok := stored.Profile(id)
	if !ok {
		t.Fatal("profile vanished after update")
	}
	if p.S3.SecretAccessKey != "ORIGINAL" {
		t.Fatalf("secret = %q, want the stored value kept", p.S3.SecretAccessKey)
	}
	if p.S3.AccessKeyId != "AK2" || p.Name != "private (renamed)" {
		t.Fatalf("non-secret fields must update: %+v", p)
	}
}

func TestTestStorageConnectionMergesStoredSecret(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, tester := newSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateStorageProfile(ctx, connect.NewRequest(&systemv1.CreateStorageProfileRequest{
		Profile: &systemv1.StorageProfile{
			Name:     "private",
			Provider: "s3",
			S3: &systemv1.S3StorageConfig{
				Endpoint:        "s3.example.com",
				Bucket:          "files",
				AccessKeyId:     "AK",
				SecretAccessKey: "STORED-SECRET",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := svc.TestStorageConnection(ctx, connect.NewRequest(&systemv1.TestStorageConnectionRequest{
		Profile: &systemv1.StorageProfile{
			Id:       created.Msg.GetProfile().GetId(),
			Provider: "s3",
			S3: &systemv1.S3StorageConfig{
				Endpoint:    "s3.example.com",
				Bucket:      "files",
				AccessKeyId: "AK",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Msg.GetOk() {
		t.Fatalf("test failed: %s", resp.Msg.GetMessage())
	}
	if tester.lastTested.S3.SecretAccessKey != "STORED-SECRET" {
		t.Fatalf("tested secret = %q, want stored secret merged in", tester.lastTested.S3.SecretAccessKey)
	}
}

func TestSnapshotEmitsBindingsInCatalogOrder(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newSystemService(q)

	resp, err := svc.GetSystemSettings(t.Context(), connect.NewRequest(&systemv1.GetSystemSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	bindings := resp.Msg.GetStorage().GetBindings()
	if len(bindings) != len(stg.Purposes) {
		t.Fatalf("bindings = %d, want one per purpose (%d)", len(bindings), len(stg.Purposes))
	}
	for i, p := range stg.Purposes {
		if bindings[i].GetPurpose() != p {
			t.Fatalf("bindings[%d] = %q, want %q", i, bindings[i].GetPurpose(), p)
		}
	}
	llmBindings := resp.Msg.GetLlm().GetBindings()
	if len(llmBindings) != len(llm.Purposes) {
		t.Fatalf("llm bindings = %d, want one per purpose (%d)", len(llmBindings), len(llm.Purposes))
	}
	for i, p := range llm.Purposes {
		if llmBindings[i].GetPurpose() != p {
			t.Fatalf("llm bindings[%d] = %q, want %q", i, llmBindings[i].GetPurpose(), p)
		}
	}
}

type recordingChatter struct{ lastProfile systemcodec.LlmProfile }

func (c *recordingChatter) Complete(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (c *recordingChatter) CompleteWith(_ context.Context, p systemcodec.LlmProfile, _, _ string) (string, error) {
	c.lastProfile = p
	return "hello", nil
}

func newLlmSystemService(q repository.Querier) (*SystemService, *recordingChatter) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chatter := &recordingChatter{}
	return NewSystemService(settings.NewStore(q), nil, nil, nil, nil, chatter, logger), chatter
}

func TestLlmProfileCRUDAndBinding(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newLlmSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateLlmProfile(ctx, connect.NewRequest(&systemv1.CreateLlmProfileRequest{
		Profile: &systemv1.LlmProfile{
			Name:     "fast",
			Provider: "openai",
			Openai:   &systemv1.OpenAiLlmConfig{ApiKey: "SECRET", Model: "gpt-4o-mini"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	profile := created.Msg.GetProfile()
	if profile.GetId() == "" {
		t.Fatal("create must assign an id")
	}
	if profile.GetOpenai().GetApiKey() != "" {
		t.Fatal("secret must never be echoed back")
	}
	if !profile.GetOpenai().GetApiKeySet() {
		t.Fatal("secret_set must report a stored secret")
	}

	if _, err := svc.BindLlmPurpose(ctx, connect.NewRequest(&systemv1.BindLlmPurposeRequest{
		Purpose:   llm.PurposeChat,
		ProfileId: profile.GetId(),
	})); err != nil {
		t.Fatal(err)
	}

	_, err = svc.BindLlmPurpose(ctx, connect.NewRequest(&systemv1.BindLlmPurposeRequest{
		Purpose:   "not-a-purpose",
		ProfileId: profile.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown purpose: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	_, err = svc.DeleteLlmProfile(ctx, connect.NewRequest(&systemv1.DeleteLlmProfileRequest{
		Id: profile.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("delete bound profile: code = %v, want failed_precondition", connect.CodeOf(err))
	}

	if _, err := svc.BindLlmPurpose(ctx, connect.NewRequest(&systemv1.BindLlmPurposeRequest{
		Purpose:   llm.PurposeChat,
		ProfileId: "",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteLlmProfile(ctx, connect.NewRequest(&systemv1.DeleteLlmProfileRequest{
		Id: profile.GetId(),
	})); err != nil {
		t.Fatalf("delete after unbind: %v", err)
	}

	var stored settings.Llm
	if err := json.Unmarshal(q.rows["llm"], &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Profiles) != 0 {
		t.Fatalf("profiles after delete = %d, want 0", len(stored.Profiles))
	}
}

func TestUpdateLlmProfileKeepsSecretWhenEmpty(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, _ := newLlmSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateLlmProfile(ctx, connect.NewRequest(&systemv1.CreateLlmProfileRequest{
		Profile: &systemv1.LlmProfile{
			Name:     "fast",
			Provider: "openai",
			Openai:   &systemv1.OpenAiLlmConfig{ApiKey: "ORIGINAL", Model: "gpt-4o-mini"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.GetProfile().GetId()

	if _, err := svc.UpdateLlmProfile(ctx, connect.NewRequest(&systemv1.UpdateLlmProfileRequest{
		Profile: &systemv1.LlmProfile{
			Id:       id,
			Name:     "fast (renamed)",
			Provider: "openai",
			Openai:   &systemv1.OpenAiLlmConfig{Model: "gpt-4o"},
		},
	})); err != nil {
		t.Fatal(err)
	}

	var stored settings.Llm
	if err := json.Unmarshal(q.rows["llm"], &stored); err != nil {
		t.Fatal(err)
	}
	p, ok := stored.Profile(id)
	if !ok {
		t.Fatal("profile vanished after update")
	}
	if p.Openai.ApiKey != "ORIGINAL" {
		t.Fatalf("secret = %q, want the stored value kept", p.Openai.ApiKey)
	}
	if p.Openai.Model != "gpt-4o" || p.Name != "fast (renamed)" {
		t.Fatalf("non-secret fields must update: %+v", p)
	}
}

func TestTestLlmMergesStoredSecret(t *testing.T) {
	q := newMemSettingsQuerier()
	svc, chatter := newLlmSystemService(q)
	ctx := t.Context()

	created, err := svc.CreateLlmProfile(ctx, connect.NewRequest(&systemv1.CreateLlmProfileRequest{
		Profile: &systemv1.LlmProfile{
			Name:     "fast",
			Provider: "openai",
			Openai:   &systemv1.OpenAiLlmConfig{ApiKey: "STORED-SECRET", Model: "gpt-4o-mini"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := svc.TestLlm(ctx, connect.NewRequest(&systemv1.TestLlmRequest{
		Profile: &systemv1.LlmProfile{
			Id:       created.Msg.GetProfile().GetId(),
			Provider: "openai",
			Openai:   &systemv1.OpenAiLlmConfig{Model: "gpt-4o-mini"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Msg.GetOk() {
		t.Fatalf("test failed: %s", resp.Msg.GetMessage())
	}
	if chatter.lastProfile.Openai.ApiKey != "STORED-SECRET" {
		t.Fatalf("tested secret = %q, want stored secret merged in", chatter.lastProfile.Openai.ApiKey)
	}
}

type memIdentityQuerier struct {
	*memSettingsQuerier
	identityCounts map[string]int64
}

func (m *memIdentityQuerier) CountIdentitiesByProvider(_ context.Context, provider string) (int64, error) {
	return m.identityCounts[provider], nil
}

func newOauthSystemService() (*SystemService, *memIdentityQuerier) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := &memIdentityQuerier{
		memSettingsQuerier: newMemSettingsQuerier(),
		identityCounts:     map[string]int64{},
	}
	return NewSystemService(settings.NewStore(q), q, nil, nil, nil, nil, logger), q
}

func TestOauthLoginBinding(t *testing.T) {
	svc, _ := newOauthSystemService()
	ctx := t.Context()

	created, err := svc.CreateOauthProfile(ctx, connect.NewRequest(&systemv1.CreateOauthProfileRequest{
		Profile: &systemv1.OauthProfile{
			Key:      "google",
			Name:     "Google",
			Provider: "oidc",
			Oidc: &systemv1.OidcOauthConfig{
				Issuer:       "https://accounts.google.com",
				ClientId:     "cid",
				ClientSecret: "SECRET",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Msg.GetProfile().GetId()

	snapshot, err := svc.GetSystemSettings(ctx, connect.NewRequest(&systemv1.GetSystemSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	bindings := snapshot.Msg.GetOauth().GetBindings()
	if len(bindings) != 1 || bindings[0].GetPurpose() != oauth.PurposeLogin {
		t.Fatalf("bindings = %v, want the login purpose in catalog order", bindings)
	}
	if len(bindings[0].GetProfileIds()) != 0 {
		t.Fatal("a fresh profile must not be bound to the sign-in page")
	}

	bound, err := svc.BindOauthPurpose(ctx, connect.NewRequest(&systemv1.BindOauthPurposeRequest{
		Purpose:    oauth.PurposeLogin,
		ProfileIds: []string{id},
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := bound.Msg.GetOauth().GetBindings()[0].GetProfileIds()
	if len(got) != 1 || got[0] != id {
		t.Fatalf("bound ids = %v, want [%s]", got, id)
	}

	_, err = svc.DeleteOauthProfile(ctx, connect.NewRequest(&systemv1.DeleteOauthProfileRequest{Id: id}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("delete bound profile: code = %v, want failed_precondition", connect.CodeOf(err))
	}

	_, err = svc.BindOauthPurpose(ctx, connect.NewRequest(&systemv1.BindOauthPurposeRequest{
		Purpose:    "not-a-purpose",
		ProfileIds: []string{id},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown purpose: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := svc.BindOauthPurpose(ctx, connect.NewRequest(&systemv1.BindOauthPurposeRequest{
		Purpose: oauth.PurposeLogin,
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteOauthProfile(ctx, connect.NewRequest(&systemv1.DeleteOauthProfileRequest{Id: id})); err != nil {
		t.Fatalf("delete after unbind: %v", err)
	}
}

func TestOauthProfileCRUD(t *testing.T) {
	svc, q := newOauthSystemService()
	ctx := t.Context()

	created, err := svc.CreateOauthProfile(ctx, connect.NewRequest(&systemv1.CreateOauthProfileRequest{
		Profile: &systemv1.OauthProfile{
			Key:      "google",
			Name:     "Google",
			Provider: "oidc",
			Oidc: &systemv1.OidcOauthConfig{
				Issuer:       "https://accounts.google.com",
				ClientId:     "cid",
				ClientSecret: "SECRET",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	profile := created.Msg.GetProfile()
	if profile.GetId() == "" {
		t.Fatal("create must assign an id")
	}
	if profile.GetOidc().GetClientSecret() != "" {
		t.Fatal("secret must never be echoed back")
	}
	if !profile.GetOidc().GetClientSecretSet() {
		t.Fatal("secret_set must report a stored secret")
	}

	_, err = svc.CreateOauthProfile(ctx, connect.NewRequest(&systemv1.CreateOauthProfileRequest{
		Profile: &systemv1.OauthProfile{
			Key:      "google",
			Name:     "Google again",
			Provider: "oidc",
		},
	}))
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("duplicate key: code = %v, want already_exists", connect.CodeOf(err))
	}

	updated, err := svc.UpdateOauthProfile(ctx, connect.NewRequest(&systemv1.UpdateOauthProfileRequest{
		Profile: &systemv1.OauthProfile{
			Id:       profile.GetId(),
			Key:      "renamed-key-must-be-ignored",
			Name:     "Google (renamed)",
			Provider: "oidc",
			Oidc: &systemv1.OidcOauthConfig{
				Issuer:   "https://accounts.google.com",
				ClientId: "cid2",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Msg.GetProfile().GetKey() != "google" {
		t.Fatalf("key = %q, want immutable %q", updated.Msg.GetProfile().GetKey(), "google")
	}

	var stored settings.OAuth
	if err := json.Unmarshal(q.rows["oauth"], &stored); err != nil {
		t.Fatal(err)
	}
	p, ok := settings.ProfileByKey(stored, "google")
	if !ok {
		t.Fatal("profile vanished after update")
	}
	if p.Oidc.ClientSecret != "SECRET" {
		t.Fatalf("secret = %q, want the stored value kept", p.Oidc.ClientSecret)
	}

	q.identityCounts["google"] = 3
	_, err = svc.DeleteOauthProfile(ctx, connect.NewRequest(&systemv1.DeleteOauthProfileRequest{
		Id: profile.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("delete with identities: code = %v, want failed_precondition", connect.CodeOf(err))
	}

	q.identityCounts["google"] = 0
	if _, err := svc.DeleteOauthProfile(ctx, connect.NewRequest(&systemv1.DeleteOauthProfileRequest{
		Id: profile.GetId(),
	})); err != nil {
		t.Fatalf("delete without identities: %v", err)
	}
}
