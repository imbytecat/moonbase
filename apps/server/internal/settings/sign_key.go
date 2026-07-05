package settings

import (
	"context"
	"crypto/rand"
	"fmt"
)

// StorageSignKey returns the HMAC secret behind local-storage signed URLs,
// generating and persisting a random one on first use so deployments need no
// extra configuration. Concurrent first calls may both write; last write
// wins and only invalidates in-flight URLs once, which is harmless.
func (s *Store) StorageSignKey(ctx context.Context) ([]byte, error) {
	return s.signKey(ctx, keyStorageSignKey)
}

// CaptchaAltchaKey returns the HMAC secret signing built-in ALTCHA captcha
// challenges, with the same generate-on-first-use lifecycle.
func (s *Store) CaptchaAltchaKey(ctx context.Context) ([]byte, error) {
	return s.signKey(ctx, keyCaptchaAltchaKey)
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
	if err := s.set(ctx, key, v); err != nil {
		return nil, err
	}
	return v, nil
}
