package captcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"

	altcha "github.com/altcha-org/altcha-lib-go"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

type fakeQuerier struct {
	repository.Querier
	values map[string]json.RawMessage
}

func (f *fakeQuerier) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	raw, ok := f.values[key]
	if !ok {
		return repository.Setting{}, pgx.ErrNoRows
	}
	return repository.Setting{Key: key, Value: raw}, nil
}

func (f *fakeQuerier) UpsertSetting(_ context.Context, arg repository.UpsertSettingParams) error {
	f.values[arg.Key] = arg.Value
	return nil
}

func newAltchaTestClient(t *testing.T) *Client {
	t.Helper()
	store := settings.NewStore(&fakeQuerier{values: map[string]json.RawMessage{}})
	client := NewClient(store)
	cfg := settings.Captcha{
		Profiles: []systemcodec.CaptchaProfile{{
			Id:       "p1",
			Name:     "altcha",
			Provider: "altcha",
			Altcha:   systemcodec.AltchaCaptchaConfig{Difficulty: 1000},
		}},
		Bindings: map[string][]string{PurposeAuth: {"p1"}},
	}
	if err := store.SetCaptcha(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	return client
}

func solveAltchaChallenge(t *testing.T, client *Client) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/captcha/altcha/challenge?purpose=auth", nil)
	client.ServeAltchaChallenge(rec, req)
	if rec.Code != 200 {
		t.Fatalf("challenge status = %d", rec.Code)
	}
	var ch altcha.Challenge
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil {
		t.Fatal(err)
	}
	sol, err := altcha.SolveChallenge(ch.Challenge, ch.Salt, altcha.SHA256, int(ch.MaxNumber), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(altcha.Payload{
		Algorithm: ch.Algorithm,
		Challenge: ch.Challenge,
		Number:    int64(sol.Number),
		Salt:      ch.Salt,
		Signature: ch.Signature,
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(payload)
}

func TestAltchaChallengeRoundTrip(t *testing.T) {
	client := newAltchaTestClient(t)
	token := solveAltchaChallenge(t, client)

	if err := client.Verify(t.Context(), PurposeAuth, token, ""); err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if err := client.Verify(t.Context(), PurposeAuth, token, ""); err == nil {
		t.Fatal("replayed token must be rejected")
	}
}

func TestAltchaRejectsWrongSolution(t *testing.T) {
	client := newAltchaTestClient(t)
	token := solveAltchaChallenge(t, client)

	raw, _ := base64.StdEncoding.DecodeString(token)
	var payload altcha.Payload
	_ = json.Unmarshal(raw, &payload)
	payload.Number++
	tampered, _ := json.Marshal(payload)

	err := client.Verify(t.Context(), PurposeAuth, base64.StdEncoding.EncodeToString(tampered), "")
	if err == nil {
		t.Fatal("tampered solution must be rejected")
	}
}

func TestAltchaChallengeUnboundPurposeIs404(t *testing.T) {
	store := settings.NewStore(&fakeQuerier{values: map[string]json.RawMessage{}})
	client := NewClient(store)

	rec := httptest.NewRecorder()
	client.ServeAltchaChallenge(rec, httptest.NewRequest("GET", "/captcha/altcha/challenge?purpose=auth", nil))
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
