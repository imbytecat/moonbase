// Package verify implements short-lived secret flows (email links, SMS codes)
// on top of the verification_tokens table. Secrets are stored hashed, are
// single-use, expire, allow limited attempts (6-digit codes are brute-forceable
// otherwise) and are rate-limited per target to bound channel spend.
package verify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

type Purpose string

const (
	PurposeEmailVerify   Purpose = "email_verify"
	PurposePasswordReset Purpose = "password_reset"
	PurposePhoneBind     Purpose = "phone_bind"
	PurposeSmsLogin      Purpose = "sms_login"
	PurposePhoneRegister Purpose = "phone_register"
	PurposeEmailRegister Purpose = "email_register"
	PurposeEmailBind     Purpose = "email_bind"
)

var (
	// ErrRateLimited: too many sends to this target in the window.
	ErrRateLimited = errors.New("too many requests, try again later")
	// ErrInvalid covers unknown/expired/consumed/attempts-exhausted secrets —
	// callers must not distinguish (that would leak state to an attacker).
	ErrInvalid = errors.New("invalid or expired code")
)

const (
	codeTTL     = 5 * time.Minute
	linkTTL     = 24 * time.Hour
	resetTTL    = 1 * time.Hour
	maxAttempts = 5
	// Per target: at most 1 send per minute and 5 per hour.
	cooldown    = 1 * time.Minute
	hourlyLimit = 5
)

type Service struct {
	repo repository.Querier
}

func NewService(repo repository.Querier) *Service {
	return &Service{repo: repo}
}

// IssueCode creates a 6-digit code for SMS flows. userID may be uuid.Nil for
// pre-login flows.
func (s *Service) IssueCode(
	ctx context.Context,
	purpose Purpose,
	target string,
	userID uuid.UUID,
) (string, error) {
	if err := s.checkRate(ctx, purpose, target); err != nil {
		return "", err
	}
	code, err := randomDigits(6)
	if err != nil {
		return "", err
	}
	if err := s.store(ctx, purpose, target, userID, code, codeTTL); err != nil {
		return "", err
	}
	return code, nil
}

// IssueLinkToken creates a URL-safe token for email flows.
func (s *Service) IssueLinkToken(
	ctx context.Context,
	purpose Purpose,
	target string,
	userID uuid.UUID,
) (string, error) {
	if err := s.checkRate(ctx, purpose, target); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	ttl := linkTTL
	if purpose == PurposePasswordReset {
		ttl = resetTTL
	}
	if err := s.store(ctx, purpose, target, userID, token, ttl); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeCode validates a code sent to target: newest active token wins,
// wrong guesses count toward maxAttempts, success consumes the token.
func (s *Service) ConsumeCode(
	ctx context.Context,
	purpose Purpose,
	target, code string,
) (repository.VerificationToken, error) {
	row, err := s.repo.GetActiveVerificationToken(ctx, repository.GetActiveVerificationTokenParams{
		Purpose: string(purpose),
		Target:  target,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.VerificationToken{}, ErrInvalid
	}
	if err != nil {
		return repository.VerificationToken{}, fmt.Errorf("load token: %w", err)
	}
	if row.Attempts >= maxAttempts {
		return repository.VerificationToken{}, ErrInvalid
	}
	if !hashEqual(code, row.SecretHash) {
		if _, err := s.repo.IncrementVerificationAttempts(ctx, row.ID); err != nil {
			return repository.VerificationToken{}, fmt.Errorf("count attempt: %w", err)
		}
		return repository.VerificationToken{}, ErrInvalid
	}
	if err := s.repo.ConsumeVerificationToken(ctx, row.ID); err != nil {
		return repository.VerificationToken{}, fmt.Errorf("consume token: %w", err)
	}
	return row, nil
}

// ConsumeLinkToken validates a URL token (looked up by its hash — the token
// itself is high-entropy, so no attempt counting is needed) and consumes it.
func (s *Service) ConsumeLinkToken(
	ctx context.Context,
	purpose Purpose,
	token string,
) (repository.VerificationToken, error) {
	row, err := s.repo.GetVerificationTokenBySecret(
		ctx,
		repository.GetVerificationTokenBySecretParams{
			Purpose:    string(purpose),
			SecretHash: hashSecret(token),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.VerificationToken{}, ErrInvalid
	}
	if err != nil {
		return repository.VerificationToken{}, fmt.Errorf("load token: %w", err)
	}
	if err := s.repo.ConsumeVerificationToken(ctx, row.ID); err != nil {
		return repository.VerificationToken{}, fmt.Errorf("consume token: %w", err)
	}
	return row, nil
}

func (s *Service) checkRate(ctx context.Context, purpose Purpose, target string) error {
	recent, err := s.repo.CountRecentVerificationTokens(
		ctx,
		repository.CountRecentVerificationTokensParams{
			Purpose:   string(purpose),
			Target:    target,
			CreatedAt: time.Now().Add(-cooldown),
		},
	)
	if err != nil {
		return fmt.Errorf("rate check: %w", err)
	}
	if recent > 0 {
		return ErrRateLimited
	}
	hourly, err := s.repo.CountRecentVerificationTokens(
		ctx,
		repository.CountRecentVerificationTokensParams{
			Purpose:   string(purpose),
			Target:    target,
			CreatedAt: time.Now().Add(-time.Hour),
		},
	)
	if err != nil {
		return fmt.Errorf("rate check: %w", err)
	}
	if hourly >= hourlyLimit {
		return ErrRateLimited
	}
	return nil
}

func (s *Service) store(
	ctx context.Context,
	purpose Purpose,
	target string,
	userID uuid.UUID,
	secret string,
	ttl time.Duration,
) error {
	_, err := s.repo.CreateVerificationToken(ctx, repository.CreateVerificationTokenParams{
		Purpose:    string(purpose),
		Target:     target,
		UserID:     pgtype.UUID{Bytes: userID, Valid: userID != uuid.Nil},
		SecretHash: hashSecret(secret),
		ExpiresAt:  time.Now().Add(ttl),
	})
	if err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	return nil
}

func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func hashEqual(secret string, hash []byte) bool {
	return subtle.ConstantTimeCompare(hashSecret(secret), hash) == 1
}

func randomDigits(n int) (string, error) {
	out := make([]byte, n)
	for i := range out {
		// crypto/rand.Int does rejection sampling — no modulo bias.
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generate code: %w", err)
		}
		out[i] = byte('0' + d.Int64())
	}
	return string(out), nil
}
