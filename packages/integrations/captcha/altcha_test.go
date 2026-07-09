package captcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"testing"

	altcha "github.com/altcha-org/altcha-lib-go"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

type fakeStore struct {
	cfg Config
	key []byte
}

func (f fakeStore) Captcha(context.Context) (Config, error) {
	return f.cfg, nil
}

func (f fakeStore) CaptchaAltchaKey(context.Context) ([]byte, error) {
	return f.key, nil
}

func newAltchaTestClient(t *testing.T) *Client {
	t.Helper()
	cfg := kitsettings.Integration[kitsettings.GenericProfile]{
		Profiles: []kitsettings.GenericProfile{{
			Id:       "p1",
			Name:     "altcha",
			Provider: "altcha",
			Config:   map[string]any{"difficulty": 1000},
		}},
		Bindings: map[string][]string{PurposeAuth: {"p1"}},
	}
	return NewClient(fakeStore{cfg: cfg, key: []byte("test-altcha-hmac-key")})
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
	client := NewClient(fakeStore{key: []byte("test-altcha-hmac-key")})

	rec := httptest.NewRecorder()
	client.ServeAltchaChallenge(rec, httptest.NewRequest("GET", "/captcha/altcha/challenge?purpose=auth", nil))
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
