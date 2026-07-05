package rpc

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
	"github.com/imbytecat/moonbase/server/internal/verify"
)

// fakeFlowQuerier backs the SMS/phone flow tests: in-memory users keyed by
// phone, real verify.Service on top of in-memory verification tokens, and a
// settings table for the region policy.
type fakeFlowQuerier struct {
	repository.Querier
	fakeStoreTokens
	usersByPhone map[string]repository.User
	phoneSet     map[uuid.UUID]string
	authSetting  []byte
}

type fakeStoreTokens struct {
	tokens      []repository.VerificationToken
	recentCount int64
}

func (f *fakeFlowQuerier) GetUserByPhone(_ context.Context, phone string) (repository.User, error) {
	u, ok := f.usersByPhone[phone]
	if !ok {
		return repository.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeFlowQuerier) SetUserPhone(_ context.Context, arg repository.SetUserPhoneParams) error {
	f.phoneSet[arg.ID] = arg.Phone
	return nil
}

func (f *fakeFlowQuerier) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	if key == "auth" && f.authSetting != nil {
		return repository.Setting{Key: key, Value: f.authSetting}, nil
	}
	return repository.Setting{}, pgx.ErrNoRows
}

func (f *fakeFlowQuerier) GetUserMfa(context.Context, uuid.UUID) (repository.UserMfa, error) {
	return repository.UserMfa{}, pgx.ErrNoRows
}

func (f *fakeFlowQuerier) CreateVerificationToken(_ context.Context, arg repository.CreateVerificationTokenParams) (repository.VerificationToken, error) {
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

func (f *fakeFlowQuerier) GetActiveVerificationToken(_ context.Context, arg repository.GetActiveVerificationTokenParams) (repository.VerificationToken, error) {
	for i := len(f.tokens) - 1; i >= 0; i-- {
		t := f.tokens[i]
		if t.Purpose == arg.Purpose && t.Target == arg.Target && !t.ConsumedAt.Valid {
			return t, nil
		}
	}
	return repository.VerificationToken{}, pgx.ErrNoRows
}

func (f *fakeFlowQuerier) IncrementVerificationAttempts(_ context.Context, id uuid.UUID) (int32, error) {
	for i := range f.tokens {
		if f.tokens[i].ID == id {
			f.tokens[i].Attempts++
			return f.tokens[i].Attempts, nil
		}
	}
	return 0, pgx.ErrNoRows
}

func (f *fakeFlowQuerier) ConsumeVerificationToken(_ context.Context, id uuid.UUID) error {
	for i := range f.tokens {
		if f.tokens[i].ID == id {
			f.tokens[i].ConsumedAt.Valid = true
			return nil
		}
	}
	return pgx.ErrNoRows
}

func (f *fakeFlowQuerier) CountRecentVerificationTokens(_ context.Context, _ repository.CountRecentVerificationTokensParams) (int64, error) {
	return f.recentCount, nil
}

// capturingSms records what would have been sent instead of hitting a
// provider API.
type capturingSms struct {
	target string
	code   string
}

func (c *capturingSms) SendCode(_ context.Context, _, e164, code string) error {
	c.target = e164
	c.code = code
	return nil
}

func (c *capturingSms) SendCodeWith(ctx context.Context, _ systemcodec.SmsProfile, e164, code string) error {
	return c.SendCode(ctx, sms.PurposeVerification, e164, code)
}

func (c *capturingSms) SendTemplateWith(ctx context.Context, _ systemcodec.SmsProfile, _, e164, content string) error {
	return c.SendCode(ctx, sms.PurposeVerification, e164, content)
}

var _ sms.Sender = (*capturingSms)(nil)

func newFlowService(q repository.Querier, smser sms.Sender) *AuthService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAuthService(AuthServiceDeps{
		Repo:      q,
		Settings:  settings.NewStore(q),
		Objects:   noopObjectStore{},
		Captcha:   allowAllCaptcha{},
		Smser:     smser,
		Verifier:  verify.NewService(q),
		Logger:    logger,
		Policy:    auth.SessionPolicy{TTL: time.Hour, MaxLifetime: 24 * time.Hour},
		PublicURL: "http://localhost:5173",
	})
}

func newFlowQuerier() *fakeFlowQuerier {
	return &fakeFlowQuerier{
		usersByPhone: map[string]repository.User{},
		phoneSet:     map[uuid.UUID]string{},
	}
}

func TestSendPhoneBindCodeRegionPolicy(t *testing.T) {
	q := newFlowQuerier()
	q.authSetting = []byte(`{"registrationEnabled":false,"allowedPhoneRegions":["CN"]}`)
	smser := &capturingSms{}
	svc := newFlowService(q, smser)
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: uuid.New()})

	// US number while policy is CN-only → invalid_argument, nothing sent.
	_, err := svc.SendPhoneBindCode(ctx, connect.NewRequest(&authv1.SendPhoneBindCodeRequest{
		PhoneNumber: "+14155552671",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("US phone under CN policy: code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if smser.target != "" {
		t.Fatal("no SMS may be sent for a rejected region")
	}

	// CN number passes; the driver seam receives E.164 (provider-specific
	// formatting like national digits now lives inside each driver).
	if _, err := svc.SendPhoneBindCode(ctx, connect.NewRequest(&authv1.SendPhoneBindCodeRequest{
		PhoneNumber: "+8613800138000",
	})); err != nil {
		t.Fatal(err)
	}
	if smser.target != "+8613800138000" {
		t.Fatalf("send target = %q, want E.164", smser.target)
	}
}

func TestBindPhoneRejectsWrongAndForeignCodes(t *testing.T) {
	q := newFlowQuerier()
	smser := &capturingSms{}
	svc := newFlowService(q, smser)

	alice, mallory := uuid.New(), uuid.New()
	aliceCtx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: alice})
	malloryCtx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: mallory})

	if _, err := svc.SendPhoneBindCode(aliceCtx, connect.NewRequest(&authv1.SendPhoneBindCodeRequest{
		PhoneNumber: "+8613800138000",
	})); err != nil {
		t.Fatal(err)
	}
	code := smser.code

	// Wrong code → rejected.
	_, err := svc.BindPhone(aliceCtx, connect.NewRequest(&authv1.BindPhoneRequest{
		PhoneNumber: "+8613800138000",
		Code:        wrongCode(code),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("wrong code: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	// Mallory stealing Alice's code for the same phone → rejected (code is
	// bound to the issuing user).
	_, err = svc.BindPhone(malloryCtx, connect.NewRequest(&authv1.BindPhoneRequest{
		PhoneNumber: "+8613800138000",
		Code:        code,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("foreign user's code: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	// Mallory consumed Alice's token with that attempt, so re-issue for the
	// happy path (fresh code, same phone, right user).
	q.recentCount = 0
	if _, err := svc.SendPhoneBindCode(aliceCtx, connect.NewRequest(&authv1.SendPhoneBindCodeRequest{
		PhoneNumber: "+8613800138000",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BindPhone(aliceCtx, connect.NewRequest(&authv1.BindPhoneRequest{
		PhoneNumber: "+8613800138000",
		Code:        smser.code,
	})); err != nil {
		t.Fatal(err)
	}
	if q.phoneSet[alice] != "+8613800138000" {
		t.Fatalf("phone stored = %q, want E.164", q.phoneSet[alice])
	}
}

func TestSmsLoginFlow(t *testing.T) {
	q := newFlowQuerier()
	userID := uuid.New()
	q.usersByPhone["+8613800138000"] = repository.User{ID: userID, IsActive: true, Email: "u@example.com"}
	smser := &capturingSms{}
	svc := newFlowService(q, smser)

	// Unbound phone: silent ok, nothing sent (anti-enumeration).
	if _, err := svc.SendSmsLoginCode(t.Context(), connect.NewRequest(&authv1.SendSmsLoginCodeRequest{
		PhoneNumber: "+8613900139000",
	})); err != nil {
		t.Fatalf("unbound phone must answer ok: %v", err)
	}
	if smser.target != "" {
		t.Fatal("no SMS may go to an unbound phone")
	}

	// Bound phone: code sent; wrong code rejected; right code logs in.
	if _, err := svc.SendSmsLoginCode(t.Context(), connect.NewRequest(&authv1.SendSmsLoginCodeRequest{
		PhoneNumber: "+8613800138000",
	})); err != nil {
		t.Fatal(err)
	}
	code := smser.code

	_, err := svc.LoginWithSms(t.Context(), connect.NewRequest(&authv1.LoginWithSmsRequest{
		PhoneNumber: "+8613800138000",
		Code:        wrongCode(code),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("wrong login code: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}
