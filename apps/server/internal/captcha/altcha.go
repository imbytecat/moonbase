// The built-in "altcha" driver serves ALTCHA proof-of-work challenges
// (https://altcha.org): the official altcha-lib-go implements the protocol;
// this file only glues it to the channel model — a public challenge
// endpoint, the settings-stored HMAC key, and a replay cache (the library
// verifies solutions but replay protection is the caller's job per the
// ALTCHA docs). No external service — it works on air-gapped networks.
package captcha

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	altcha "github.com/altcha-org/altcha-lib-go"

	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

const (
	altchaDefaultMaxNumber = 1_000_000
	altchaChallengeTTL     = 10 * time.Minute
)

// ServeAltchaChallenge issues a challenge for the purpose given in the
// query string; 404 when the purpose isn't bound to an altcha profile.
// Public by design — the login page calls it before any session exists.
func (c *Client) ServeAltchaChallenge(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.store.Captcha(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p, ok := cfg.ProfileFor(r.URL.Query().Get("purpose"))
	if !ok || p.Provider != "altcha" {
		http.NotFound(w, r)
		return
	}
	challenge, err := c.newAltchaChallenge(r.Context(), p.Altcha)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(challenge)
}

func (c *Client) newAltchaChallenge(ctx context.Context, cfg systemcodec.AltchaCaptchaConfig) (*altcha.Challenge, error) {
	key, err := c.altchaHmacKey(ctx)
	if err != nil {
		return nil, err
	}
	maxNumber := int64(cfg.Difficulty)
	if maxNumber <= 0 {
		maxNumber = altchaDefaultMaxNumber
	}
	expires := time.Now().Add(altchaChallengeTTL)
	challenge, err := altcha.CreateChallenge(altcha.ChallengeOptions{
		Algorithm: altcha.SHA256,
		HMACKey:   key,
		MaxNumber: maxNumber,
		Expires:   &expires,
	})
	if err != nil {
		return nil, fmt.Errorf("create altcha challenge: %w", err)
	}
	return &challenge, nil
}

// verifyAltcha checks the widget's base64 payload with the official
// library, then consumes the token so a solved challenge can't be replayed.
func (c *Client) verifyAltcha(ctx context.Context, _ systemcodec.CaptchaProfile, token, _ string) error {
	key, err := c.altchaHmacKey(ctx)
	if err != nil {
		return err
	}
	ok, err := altcha.VerifySolutionSafe(token, key, true)
	if err != nil {
		return fmt.Errorf("altcha verify: %w", err)
	}
	if !ok {
		return fmt.Errorf("captcha verification failed")
	}
	if !c.altchaReplay.consume(token, time.Now().Add(altchaChallengeTTL)) {
		return fmt.Errorf("captcha verification failed: challenge already used")
	}
	return nil
}

func (c *Client) altchaHmacKey(ctx context.Context) (string, error) {
	key, err := c.store.CaptchaAltchaKey(ctx)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// replayCache remembers consumed tokens until their challenge expires,
// making each one single-use. In-memory is deliberate: challenges are
// short-lived, and a restart only re-allows a token that was already
// accepted once — the signature and expiry checks still hold.
type replayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newReplayCache() *replayCache {
	return &replayCache{seen: map[string]time.Time{}}
}

func (r *replayCache) consume(id string, expires time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, exp := range r.seen {
		if now.After(exp) {
			delete(r.seen, k)
		}
	}
	if _, dup := r.seen[id]; dup {
		return false
	}
	r.seen[id] = expires
	return true
}
