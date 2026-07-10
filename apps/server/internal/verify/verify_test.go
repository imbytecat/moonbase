package verify

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// fakeStore is an in-memory Querier double covering only the verification
// methods the Service touches (struct-embedding keeps the rest unimplemented).
type fakeStore struct {
	repository.Querier
	tokens      []repository.VerificationToken
	recentCount int64
}

func (f *fakeStore) CreateVerificationToken(
	_ context.Context,
	arg repository.CreateVerificationTokenParams,
) (repository.VerificationToken, error) {
	row := repository.VerificationToken{
		ID:         uuid.New(),
		Purpose:    arg.Purpose,
		Target:     arg.Target,
		UserID:     arg.UserID,
		SecretHash: arg.SecretHash,
		ExpiresAt:  arg.ExpiresAt,
	}
	f.tokens = append(f.tokens, row)
	return row, nil
}

func (f *fakeStore) GetActiveVerificationToken(
	_ context.Context,
	arg repository.GetActiveVerificationTokenParams,
) (repository.VerificationToken, error) {
	for i := len(f.tokens) - 1; i >= 0; i-- {
		t := f.tokens[i]
		if t.Purpose == arg.Purpose && t.Target == arg.Target && !t.ConsumedAt.Valid {
			return t, nil
		}
	}
	return repository.VerificationToken{}, pgx.ErrNoRows
}

func (f *fakeStore) GetVerificationTokenBySecret(
	_ context.Context,
	arg repository.GetVerificationTokenBySecretParams,
) (repository.VerificationToken, error) {
	for _, t := range f.tokens {
		if t.Purpose == arg.Purpose && string(t.SecretHash) == string(arg.SecretHash) &&
			!t.ConsumedAt.Valid {
			return t, nil
		}
	}
	return repository.VerificationToken{}, pgx.ErrNoRows
}

func (f *fakeStore) IncrementVerificationAttempts(_ context.Context, id uuid.UUID) (int32, error) {
	for i := range f.tokens {
		if f.tokens[i].ID == id {
			f.tokens[i].Attempts++
			return f.tokens[i].Attempts, nil
		}
	}
	return 0, pgx.ErrNoRows
}

func (f *fakeStore) ConsumeVerificationToken(_ context.Context, id uuid.UUID) error {
	for i := range f.tokens {
		if f.tokens[i].ID == id {
			f.tokens[i].ConsumedAt.Valid = true
			return nil
		}
	}
	return pgx.ErrNoRows
}

func (f *fakeStore) CountRecentVerificationTokens(
	_ context.Context,
	_ repository.CountRecentVerificationTokensParams,
) (int64, error) {
	return f.recentCount, nil
}

func TestIssueAndConsumeCode(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)
	userID := uuid.New()

	code, err := svc.IssueCode(t.Context(), PurposeSmsLogin, "+8613800138000", userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code = %q, want 6 digits", code)
	}
	if string(store.tokens[0].SecretHash) == code {
		t.Fatal("secret must be stored hashed, not in plaintext")
	}
	if sum := sha256.Sum256([]byte(code)); string(store.tokens[0].SecretHash) != string(sum[:]) {
		t.Fatal("stored hash must be SHA-256 of the code")
	}

	row, err := svc.ConsumeCode(t.Context(), PurposeSmsLogin, "+8613800138000", code)
	if err != nil {
		t.Fatal(err)
	}
	if uuid.UUID(row.UserID.Bytes) != userID {
		t.Fatal("consumed token must carry the issuing user")
	}

	// Single-use: the same code is dead after consumption.
	if _, err := svc.ConsumeCode(
		t.Context(),
		PurposeSmsLogin,
		"+8613800138000",
		code,
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatalf("reuse must fail with ErrInvalid, got %v", err)
	}
}

func TestConsumeCodeWrongGuessesExhaustAttempts(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	code, err := svc.IssueCode(t.Context(), PurposeSmsLogin, "+8613800138000", uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	for range maxAttempts {
		if _, err := svc.ConsumeCode(
			t.Context(),
			PurposeSmsLogin,
			"+8613800138000",
			"000000",
		); !errors.Is(
			err,
			ErrInvalid,
		) {
			t.Fatalf("wrong guess must return ErrInvalid, got %v", err)
		}
	}
	// Attempts exhausted: even the RIGHT code is now rejected.
	if _, err := svc.ConsumeCode(
		t.Context(),
		PurposeSmsLogin,
		"+8613800138000",
		code,
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatalf("exhausted token must reject the correct code, got %v", err)
	}
}

func TestIssueCodeRateLimited(t *testing.T) {
	store := &fakeStore{recentCount: 1}
	svc := NewService(store)

	if _, err := svc.IssueCode(
		t.Context(),
		PurposeSmsLogin,
		"+8613800138000",
		uuid.New(),
	); !errors.Is(
		err,
		ErrRateLimited,
	) {
		t.Fatalf("send within cooldown must be rate limited, got %v", err)
	}
}

func TestConsumeCodeUnknownTarget(t *testing.T) {
	svc := NewService(&fakeStore{})
	if _, err := svc.ConsumeCode(
		t.Context(),
		PurposeSmsLogin,
		"+8613800138000",
		"123456",
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatalf("no token for target must be ErrInvalid, got %v", err)
	}
}

func TestLinkTokenRoundTrip(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)
	userID := uuid.New()

	token, err := svc.IssueLinkToken(t.Context(), PurposePasswordReset, "user@example.com", userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("link token too short: %d chars", len(token))
	}

	row, err := svc.ConsumeLinkToken(t.Context(), PurposePasswordReset, token)
	if err != nil {
		t.Fatal(err)
	}
	if uuid.UUID(row.UserID.Bytes) != userID {
		t.Fatal("consumed token must carry the issuing user")
	}
	if _, err := svc.ConsumeLinkToken(
		t.Context(),
		PurposePasswordReset,
		token,
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatalf("link token must be single-use, got %v", err)
	}
	// Wrong purpose never matches even with the right secret.
	token2, err := svc.IssueLinkToken(t.Context(), PurposeEmailVerify, "user@example.com", userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConsumeLinkToken(
		t.Context(),
		PurposePasswordReset,
		token2,
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatalf("cross-purpose consumption must fail, got %v", err)
	}
}
