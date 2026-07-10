package settings

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// StorageSignKey returns the HMAC secret behind local-storage signed URLs,
// generating and persisting a random one on first use so deployments need no
// extra configuration. The database get-or-create keeps concurrent first
// calls on one stable key.
func (s *Store) StorageSignKey(ctx context.Context) ([]byte, error) {
	return s.signKey(ctx, keyStorageSignKey)
}

// CaptchaAltchaKey returns the HMAC secret signing built-in ALTCHA captcha
// challenges, with the same generate-on-first-use lifecycle.
func (s *Store) CaptchaAltchaKey(ctx context.Context) ([]byte, error) {
	return s.signKey(ctx, keyCaptchaAltchaKey)
}

// PaymentCheckoutSignKey signs high-entropy checkout session identifiers.
// The key is generated once and persisted in settings like other local HMAC
// seams, so issued checkout URLs survive process restarts.
func (s *Store) PaymentCheckoutSignKey(ctx context.Context) ([]byte, error) {
	return s.signKey(ctx, keyPaymentCheckoutSignKey)
}

func (s *Store) signKey(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	if err := s.get(ctx, key, &v); err != nil {
		return nil, err
	}
	if len(v) > 0 {
		return v, nil
	}
	v = make([]byte, 32)
	if _, err := rand.Read(v); err != nil {
		return nil, fmt.Errorf("generate sign key %s: %w", key, err)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode sign key %s: %w", key, err)
	}
	row, err := s.repo.GetOrCreateSetting(ctx, repository.GetOrCreateSettingParams{Key: key, Value: raw})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.Value, &v); err != nil {
		return nil, fmt.Errorf("decode sign key %s: %w", key, err)
	}
	return v, nil
}
