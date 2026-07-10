package pay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

var (
	ErrIdempotencyConflict = errors.New("checkout idempotency key belongs to a different command")
	ErrInvalidCheckout     = errors.New("invalid checkout session")
	ErrCheckoutExpired     = errors.New("checkout session expired")
)

type CheckoutCommand struct {
	Purpose           string
	BusinessReference string
	IdempotencyKey    string
	Subject           string
	Amount            int64
	ReturnPath        string
}

type IssuedCheckout struct {
	CheckoutURL string
	Token       string
}

type CheckoutIssuer interface {
	Create(ctx context.Context, command CheckoutCommand) (IssuedCheckout, error)
}

type CheckoutManager struct {
	repo      repository.Querier
	settings  *settings.Store
	publicURL string
	now       func() time.Time
}

func NewCheckoutIssuer(
	repo repository.Querier,
	store *settings.Store,
	publicURL string,
) *CheckoutManager {
	return &CheckoutManager{
		repo: repo, settings: store, publicURL: strings.TrimSuffix(publicURL, "/"), now: time.Now,
	}
}

func (i *CheckoutManager) Create(
	ctx context.Context,
	command CheckoutCommand,
) (IssuedCheckout, error) {
	if !Purposes.Known(command.Purpose) {
		return IssuedCheckout{}, fmt.Errorf("unknown payment purpose %q", command.Purpose)
	}
	if command.BusinessReference == "" || command.IdempotencyKey == "" || command.Subject == "" ||
		command.Amount <= 0 {
		return IssuedCheckout{}, fmt.Errorf("invalid checkout command")
	}
	if !validReturnPath(command.ReturnPath) {
		return IssuedCheckout{}, fmt.Errorf("invalid checkout return path")
	}
	hash, err := checkoutCommandHash(command)
	if err != nil {
		return IssuedCheckout{}, err
	}
	id, err := randomSessionID()
	if err != nil {
		return IssuedCheckout{}, err
	}
	row, err := i.repo.InsertCheckoutSession(ctx, repository.InsertCheckoutSessionParams{
		ID: id, Purpose: command.Purpose, BusinessReference: command.BusinessReference,
		IdempotencyKey: command.IdempotencyKey, CommandHash: hash,
		Subject: command.Subject, Amount: command.Amount, Currency: "CNY",
		ReturnPath: command.ReturnPath, ExpiresAt: i.now().Add(30 * time.Minute),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = i.repo.GetCheckoutSessionByIdempotency(
			ctx,
			repository.GetCheckoutSessionByIdempotencyParams{
				Purpose: command.Purpose, IdempotencyKey: command.IdempotencyKey,
			},
		)
	}
	if err != nil {
		return IssuedCheckout{}, fmt.Errorf("create checkout session: %w", err)
	}
	if !hmac.Equal(row.CommandHash, hash) {
		return IssuedCheckout{}, ErrIdempotencyConflict
	}
	token, err := i.Token(ctx, row.ID)
	if err != nil {
		return IssuedCheckout{}, err
	}
	return IssuedCheckout{CheckoutURL: i.publicURL + "/checkout/" + token, Token: token}, nil
}

func (i *CheckoutManager) Resolve(
	ctx context.Context,
	token string,
) (repository.PaymentCheckoutSession, error) {
	id, signature, ok := strings.Cut(token, ".")
	if !ok || id == "" || signature == "" {
		return repository.PaymentCheckoutSession{}, ErrInvalidCheckout
	}
	key, err := i.settings.PaymentCheckoutSignKey(ctx)
	if err != nil {
		return repository.PaymentCheckoutSession{}, err
	}
	want := checkoutSignature(key, id)
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(got, want) {
		return repository.PaymentCheckoutSession{}, ErrInvalidCheckout
	}
	row, err := i.repo.GetCheckoutSession(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.PaymentCheckoutSession{}, ErrInvalidCheckout
	}
	if err != nil {
		return repository.PaymentCheckoutSession{}, err
	}
	if !row.ExpiresAt.After(i.now()) {
		return repository.PaymentCheckoutSession{}, ErrCheckoutExpired
	}
	return row, nil
}

func (i *CheckoutManager) Token(ctx context.Context, id string) (string, error) {
	key, err := i.settings.PaymentCheckoutSignKey(ctx)
	if err != nil {
		return "", err
	}
	return id + "." + base64.RawURLEncoding.EncodeToString(checkoutSignature(key, id)), nil
}

func (i *CheckoutManager) CheckoutURL(token string) string {
	return i.publicURL + "/checkout/" + token
}

func (i *CheckoutManager) ReturnURL(path string) string { return i.publicURL + path }

func checkoutSignature(key []byte, id string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id))
	return mac.Sum(nil)
}

func checkoutCommandHash(command CheckoutCommand) ([]byte, error) {
	raw, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("encode checkout command: %w", err)
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

func randomSessionID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate checkout session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func validReturnPath(path string) bool {
	parsed, err := url.Parse(path)
	return err == nil && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") &&
		!parsed.IsAbs() &&
		parsed.Host == ""
}
