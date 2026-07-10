package pay

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

type issuerRepo struct {
	repository.Querier
	mu               sync.Mutex
	settings         map[string][]byte
	sessions         map[string]repository.PaymentCheckoutSession
	byKey            map[string]string
	syncFirstSignKey bool
	missingKeyReads  int
	releaseKeyReads  chan struct{}
}

func newIssuerRepo() *issuerRepo {
	return &issuerRepo{
		settings: map[string][]byte{}, sessions: map[string]repository.PaymentCheckoutSession{},
		byKey: map[string]string{}, releaseKeyReads: make(chan struct{}),
	}
}

func (r *issuerRepo) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	r.mu.Lock()
	value, ok := r.settings[key]
	if ok {
		r.mu.Unlock()
		return repository.Setting{Key: key, Value: bytes.Clone(value)}, nil
	}
	if r.syncFirstSignKey {
		r.missingKeyReads++
		if r.missingKeyReads == 2 {
			close(r.releaseKeyReads)
		}
		release := r.releaseKeyReads
		r.mu.Unlock()
		<-release
		return repository.Setting{}, pgx.ErrNoRows
	}
	r.mu.Unlock()
	return repository.Setting{}, pgx.ErrNoRows
}

func (r *issuerRepo) UpsertSetting(_ context.Context, arg repository.UpsertSettingParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings[arg.Key] = bytes.Clone(arg.Value)
	return nil
}

func (r *issuerRepo) GetOrCreateSetting(
	_ context.Context,
	arg repository.GetOrCreateSettingParams,
) (repository.Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok := r.settings[arg.Key]; ok {
		return repository.Setting{Key: arg.Key, Value: bytes.Clone(value)}, nil
	}
	r.settings[arg.Key] = bytes.Clone(arg.Value)
	return repository.Setting{Key: arg.Key, Value: bytes.Clone(arg.Value)}, nil
}

func (r *issuerRepo) InsertCheckoutSession(
	_ context.Context,
	arg repository.InsertCheckoutSessionParams,
) (repository.PaymentCheckoutSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := arg.Purpose + "\x00" + arg.IdempotencyKey
	if _, exists := r.byKey[key]; exists {
		return repository.PaymentCheckoutSession{}, pgx.ErrNoRows
	}
	row := repository.PaymentCheckoutSession{
		ID: arg.ID, Purpose: arg.Purpose, BusinessReference: arg.BusinessReference,
		IdempotencyKey: arg.IdempotencyKey, CommandHash: bytes.Clone(arg.CommandHash),
		Subject: arg.Subject, Amount: arg.Amount, Currency: arg.Currency,
		ReturnPath: arg.ReturnPath, Status: "open", ExpiresAt: arg.ExpiresAt,
	}
	r.sessions[row.ID] = row
	r.byKey[key] = row.ID
	return row, nil
}

func (r *issuerRepo) GetCheckoutSessionByIdempotency(
	_ context.Context,
	arg repository.GetCheckoutSessionByIdempotencyParams,
) (repository.PaymentCheckoutSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byKey[arg.Purpose+"\x00"+arg.IdempotencyKey]
	if !ok {
		return repository.PaymentCheckoutSession{}, pgx.ErrNoRows
	}
	return r.sessions[id], nil
}

func (r *issuerRepo) GetCheckoutSession(
	_ context.Context,
	id string,
) (repository.PaymentCheckoutSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.sessions[id]
	if !ok {
		return repository.PaymentCheckoutSession{}, pgx.ErrNoRows
	}
	return row, nil
}

func TestCheckoutIssuerConcurrentFirstUseKeepsEveryURLValid(t *testing.T) {
	repo := newIssuerRepo()
	repo.syncFirstSignKey = true
	issuer := NewCheckoutIssuer(repo, settings.NewStore(repo), "https://moonbase.test")

	issued := make(chan IssuedCheckout, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkout, err := issuer.Create(context.Background(), CheckoutCommand{
				Purpose:           PurposeCheckout,
				BusinessReference: "order",
				IdempotencyKey:    string(rune('a' + i)),
				Subject:           "并发首用",
				Amount:            100,
				ReturnPath:        "/orders",
			})
			if err != nil {
				errs <- err
				return
			}
			issued <- checkout
		}()
	}
	wg.Wait()
	close(issued)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for checkout := range issued {
		if _, err := issuer.Resolve(t.Context(), checkout.Token); err != nil {
			t.Fatalf("Resolve(concurrently issued token) = %v", err)
		}
	}
}

func TestCheckoutIssuerIsIdempotentAndRejectsCommandDrift(t *testing.T) {
	repo := newIssuerRepo()
	issuer := NewCheckoutIssuer(repo, settings.NewStore(repo), "https://moonbase.test")
	command := CheckoutCommand{
		Purpose: PurposeCheckout, BusinessReference: "order-42", IdempotencyKey: "attempt-1",
		Subject: "测试订单", Amount: 199, ReturnPath: "/orders/42",
	}

	first, err := issuer.Create(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := issuer.Create(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.CheckoutURL != second.CheckoutURL {
		t.Fatalf("idempotent URLs differ: %q != %q", first.CheckoutURL, second.CheckoutURL)
	}
	if _, err := issuer.Resolve(t.Context(), first.Token); err != nil {
		t.Fatalf("Resolve(valid token) = %v", err)
	}

	drifted := command
	drifted.Amount++
	if _, err := issuer.Create(t.Context(), drifted); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Create(drifted command) = %v, want ErrIdempotencyConflict", err)
	}
}
