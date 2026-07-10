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

func (f fakeStore) Captcha(context.Context) (Config, error)          { return f.cfg, nil }
func (f fakeStore) CaptchaAltchaKey(context.Context) ([]byte, error) { return f.key, nil }
func altchaClient(t *testing.T) *Client {
	t.Helper()
	store := fakeStore{cfg: Config{Profiles: []kitsettings.GenericProfile{{Id: "p", Provider: "altcha", Config: map[string]any{"difficulty": 1000}}}, Bindings: map[string][]string{"auth": {"p"}}}, key: []byte("test-altcha-key")}
	return NewClient(store, NewRegistry(store))
}
func solve(t *testing.T, c *Client) string {
	t.Helper()
	rec := httptest.NewRecorder()
	c.ServeAltchaChallenge(rec, httptest.NewRequest("GET", "/captcha/altcha/challenge?purpose=auth", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var ch altcha.Challenge
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil {
		t.Fatal(err)
	}
	sol, err := altcha.SolveChallenge(ch.Challenge, ch.Salt, altcha.SHA256, int(ch.MaxNumber), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(altcha.Payload{Algorithm: ch.Algorithm, Challenge: ch.Challenge, Number: int64(sol.Number), Salt: ch.Salt, Signature: ch.Signature})
	return base64.StdEncoding.EncodeToString(raw)
}
func TestAltchaChallengeRoundTripAndReplayProtection(t *testing.T) {
	c := altchaClient(t)
	token := solve(t, c)
	if err := c.Verify(t.Context(), "auth", token, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(t.Context(), "auth", token, ""); err == nil {
		t.Fatal("replay must fail")
	}
}
func TestUnboundCaptchaPassesButInvalidBoundProfileFailsClosed(t *testing.T) {
	empty := fakeStore{}
	c := NewClient(empty, NewRegistry(empty))
	if err := c.Verify(t.Context(), "auth", "", ""); err != nil {
		t.Fatalf("unbound=%v", err)
	}
	bad := fakeStore{cfg: Config{Profiles: []kitsettings.GenericProfile{{Id: "p", Provider: "turnstile", Config: map[string]any{"siteKey": "site"}}}, Bindings: map[string][]string{"auth": {"p"}}}}
	c = NewClient(bad, NewRegistry(bad))
	if err := c.Verify(t.Context(), "auth", "token", ""); err == nil {
		t.Fatal("invalid bound profile must fail closed")
	}
}
