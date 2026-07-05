package rpc

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pquerna/otp/totp"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

type fakeMfaQuerier struct {
	fakeAuthQuerier
	mfa     map[uuid.UUID]repository.UserMfa
	tickets map[string]repository.OauthSignupTicket
}

func newFakeMfaQuerier() *fakeMfaQuerier {
	return &fakeMfaQuerier{
		mfa:     map[uuid.UUID]repository.UserMfa{},
		tickets: map[string]repository.OauthSignupTicket{},
	}
}

func (f *fakeMfaQuerier) GetUserMfa(_ context.Context, userID uuid.UUID) (repository.UserMfa, error) {
	row, ok := f.mfa[userID]
	if !ok {
		return repository.UserMfa{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeMfaQuerier) UpsertPendingUserMfa(_ context.Context, arg repository.UpsertPendingUserMfaParams) (int64, error) {
	if row, ok := f.mfa[arg.UserID]; ok && row.ActivatedAt.Valid {
		return 0, nil
	}
	f.mfa[arg.UserID] = repository.UserMfa{
		UserID:        arg.UserID,
		TotpSecret:    arg.TotpSecret,
		RecoveryCodes: arg.RecoveryCodes,
	}
	return 1, nil
}

func (f *fakeMfaQuerier) ActivateUserMfa(_ context.Context, userID uuid.UUID) (int64, error) {
	row, ok := f.mfa[userID]
	if !ok || row.ActivatedAt.Valid {
		return 0, nil
	}
	row.ActivatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	f.mfa[userID] = row
	return 1, nil
}

func (f *fakeMfaQuerier) DeleteUserMfa(_ context.Context, userID uuid.UUID) (int64, error) {
	if _, ok := f.mfa[userID]; !ok {
		return 0, nil
	}
	delete(f.mfa, userID)
	return 1, nil
}

func (f *fakeMfaQuerier) ConsumeMfaRecoveryCode(_ context.Context, arg repository.ConsumeMfaRecoveryCodeParams) (int64, error) {
	row, ok := f.mfa[arg.UserID]
	if !ok {
		return 0, nil
	}
	for i, h := range row.RecoveryCodes {
		if string(h) == string(arg.CodeHash) {
			row.RecoveryCodes = append(row.RecoveryCodes[:i], row.RecoveryCodes[i+1:]...)
			f.mfa[arg.UserID] = row
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeMfaQuerier) CreateOauthSignupTicket(_ context.Context, arg repository.CreateOauthSignupTicketParams) (repository.OauthSignupTicket, error) {
	row := repository.OauthSignupTicket{
		ID:         uuid.New(),
		Provider:   arg.Provider,
		ProviderID: arg.ProviderID,
		SecretHash: arg.SecretHash,
		ExpiresAt:  arg.ExpiresAt,
	}
	f.tickets[string(arg.SecretHash)] = row
	return row, nil
}

func (f *fakeMfaQuerier) ConsumeOauthSignupTicket(_ context.Context, secretHash []byte) (repository.OauthSignupTicket, error) {
	row, ok := f.tickets[string(secretHash)]
	if !ok {
		return repository.OauthSignupTicket{}, pgx.ErrNoRows
	}
	delete(f.tickets, string(secretHash))
	return row, nil
}

func TestTotpTwoStepLoginFlow(t *testing.T) {
	userID := uuid.New()
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	q := newFakeMfaQuerier()
	q.getUserByUsername = func(context.Context, string) (repository.User, error) {
		return repository.User{ID: userID, Username: "alice", PasswordHash: hash, IsActive: true}, nil
	}
	q.getUser = func(context.Context, uuid.UUID) (repository.User, error) {
		return repository.User{ID: userID, PasswordHash: hash, IsActive: true}, nil
	}
	q.getSetting = func(context.Context, string) (repository.Setting, error) {
		return repository.Setting{}, pgx.ErrNoRows
	}
	q.createSession = func(_ context.Context, arg repository.CreateSessionParams) (repository.Session, error) {
		return repository.Session{ID: uuid.New(), UserID: arg.UserID}, nil
	}
	q.getSessionIdentity = func(context.Context, []byte) (repository.GetSessionIdentityRow, error) {
		return repository.GetSessionIdentityRow{SessionID: uuid.New(), UserID: userID, Username: "alice"}, nil
	}
	svc := newAuthService(q)
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID, Username: "alice"})

	setup, err := svc.SetupTotp(ctx, connect.NewRequest(&authv1.SetupTotpRequest{
		CurrentPassword: "the-real-password",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(setup.Msg.GetRecoveryCodes()) != recoveryCodeCount {
		t.Fatalf("recovery codes = %d, want %d", len(setup.Msg.GetRecoveryCodes()), recoveryCodeCount)
	}

	// Login is single-step until the factor is CONFIRMED.
	login, err := svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "alice",
		Password:   "the-real-password",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if login.Msg.GetMfaRequired() {
		t.Fatal("unconfirmed setup must not gate login")
	}

	code, err := totp.GenerateCode(setup.Msg.GetSecret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateTotp(ctx, connect.NewRequest(&authv1.ActivateTotpRequest{Code: code})); err != nil {
		t.Fatal(err)
	}

	login, err = svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "alice",
		Password:   "the-real-password",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !login.Msg.GetMfaRequired() || login.Msg.GetMfaTicket() == "" {
		t.Fatal("confirmed factor must switch login to two steps")
	}
	if login.Msg.GetSessionToken() != "" {
		t.Fatal("no session may be issued before the second factor")
	}

	code, err = totp.GenerateCode(setup.Msg.GetSecret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.LoginWithTotp(t.Context(), connect.NewRequest(&authv1.LoginWithTotpRequest{
		MfaTicket: login.Msg.GetMfaTicket(),
		Code:      code,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if second.Msg.GetSessionToken() == "" {
		t.Fatal("second step must issue the session")
	}

	// The ticket is single-use.
	_, err = svc.LoginWithTotp(t.Context(), connect.NewRequest(&authv1.LoginWithTotpRequest{
		MfaTicket: login.Msg.GetMfaTicket(),
		Code:      code,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated (ticket reuse)", connect.CodeOf(err))
	}
}

func TestTotpRecoveryCodeSingleUse(t *testing.T) {
	userID := uuid.New()
	q := newFakeMfaQuerier()
	q.createSession = func(_ context.Context, arg repository.CreateSessionParams) (repository.Session, error) {
		return repository.Session{ID: uuid.New(), UserID: arg.UserID}, nil
	}
	q.getSessionIdentity = func(context.Context, []byte) (repository.GetSessionIdentityRow, error) {
		return repository.GetSessionIdentityRow{SessionID: uuid.New(), UserID: userID}, nil
	}
	svc := newAuthService(q)

	recovery := "abcde-fghij"
	sum := sha256.Sum256([]byte(recovery))
	q.mfa[userID] = repository.UserMfa{
		UserID:        userID,
		TotpSecret:    "JBSWY3DPEHPK3PXP",
		RecoveryCodes: [][]byte{sum[:]},
		ActivatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	issue := func() string {
		t.Helper()
		ticket, err := svc.issueMfaTicket(t.Context(), userID)
		if err != nil {
			t.Fatal(err)
		}
		return ticket
	}

	if _, err := svc.LoginWithTotp(t.Context(), connect.NewRequest(&authv1.LoginWithTotpRequest{
		MfaTicket: issue(),
		Code:      recovery,
	})); err != nil {
		t.Fatalf("recovery code must sign in: %v", err)
	}

	_, err := svc.LoginWithTotp(t.Context(), connect.NewRequest(&authv1.LoginWithTotpRequest{
		MfaTicket: issue(),
		Code:      recovery,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument (recovery code reuse)", connect.CodeOf(err))
	}
}

func TestDisableTotpRequiresPassword(t *testing.T) {
	userID := uuid.New()
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	q := newFakeMfaQuerier()
	q.getUser = func(context.Context, uuid.UUID) (repository.User, error) {
		return repository.User{ID: userID, PasswordHash: hash}, nil
	}
	q.mfa[userID] = repository.UserMfa{
		UserID:      userID,
		ActivatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	svc := newAuthService(q)
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})

	_, err = svc.DisableTotp(ctx, connect.NewRequest(&authv1.DisableTotpRequest{
		CurrentPassword: "wrong-password-1",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, ok := q.mfa[userID]; !ok {
		t.Fatal("factor must survive a failed disable")
	}

	if _, err := svc.DisableTotp(ctx, connect.NewRequest(&authv1.DisableTotpRequest{
		CurrentPassword: "the-real-password",
	})); err != nil {
		t.Fatal(err)
	}
	if _, ok := q.mfa[userID]; ok {
		t.Fatal("factor must be removed")
	}
}
