package rpc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const (
	mfaTicketTTL      = 5 * time.Minute
	recoveryCodeCount = 8
)

// SetupTotp generates a fresh secret + recovery codes for the CURRENT user.
// Password-gated (a hijacked session must not seed a factor the attacker
// controls). Overwrites any unconfirmed setup; refuses when already active.
func (s *AuthService) SetupTotp(
	ctx context.Context,
	req *connect.Request[authv1.SetupTotpRequest],
) (*connect.Response[authv1.SetupTotpResponse], error) {
	id := auth.IdentityFromContext(ctx)
	if err := s.requirePassword(ctx, id.UserID, req.Msg.GetCurrentPassword()); err != nil {
		return nil, err
	}

	issuer := "moonbase"
	if site, err := s.settings.Site(ctx); err == nil && site.Name != "" {
		issuer = site.Name
	}
	account := id.Username
	if account == "" {
		account = id.Email
	}
	if account == "" {
		account = id.Phone
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: account})
	if err != nil {
		return nil, s.internal(ctx, "generate totp secret", err)
	}

	codes := make([]string, recoveryCodeCount)
	hashes := make([][]byte, recoveryCodeCount)
	for i := range codes {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, s.internal(ctx, "generate recovery code", err)
		}
		codes[i] = code
		sum := sha256.Sum256([]byte(code))
		hashes[i] = sum[:]
	}

	rows, err := s.repo.UpsertPendingUserMfa(ctx, repository.UpsertPendingUserMfaParams{
		UserID:        id.UserID,
		TotpSecret:    key.Secret(),
		RecoveryCodes: hashes,
	})
	if err != nil {
		return nil, s.internal(ctx, "store totp setup", err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("two-factor authentication is already enabled"))
	}

	return connect.NewResponse(&authv1.SetupTotpResponse{
		OtpauthUrl:    key.URL(),
		Secret:        key.Secret(),
		RecoveryCodes: codes,
	}), nil
}

// ActivateTotp confirms the authenticator holds the secret before the factor
// starts gating logins — no lockout by mistyped setup.
func (s *AuthService) ActivateTotp(
	ctx context.Context,
	req *connect.Request[authv1.ActivateTotpRequest],
) (*connect.Response[authv1.ActivateTotpResponse], error) {
	id := auth.IdentityFromContext(ctx)
	mfa, err := s.repo.GetUserMfa(ctx, id.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("run setup first"))
	}
	if err != nil {
		return nil, s.internal(ctx, "load totp setup", err)
	}
	if mfa.ActivatedAt.Valid {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("two-factor authentication is already enabled"))
	}
	if !totp.Validate(req.Msg.GetCode(), mfa.TotpSecret) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid code"))
	}
	if _, err := s.repo.ActivateUserMfa(ctx, id.UserID); err != nil {
		return nil, s.internal(ctx, "activate totp", err)
	}
	return connect.NewResponse(&authv1.ActivateTotpResponse{}), nil
}

func (s *AuthService) DisableTotp(
	ctx context.Context,
	req *connect.Request[authv1.DisableTotpRequest],
) (*connect.Response[authv1.DisableTotpResponse], error) {
	id := auth.IdentityFromContext(ctx)
	if err := s.requirePassword(ctx, id.UserID, req.Msg.GetCurrentPassword()); err != nil {
		return nil, err
	}
	if _, err := s.repo.DeleteUserMfa(ctx, id.UserID); err != nil {
		return nil, s.internal(ctx, "disable totp", err)
	}
	return connect.NewResponse(&authv1.DisableTotpResponse{}), nil
}

// LoginWithTotp is the second login step: the ticket (issued by Login after a
// correct password) plus a TOTP or recovery code yields the session.
func (s *AuthService) LoginWithTotp(
	ctx context.Context,
	req *connect.Request[authv1.LoginWithTotpRequest],
) (*connect.Response[authv1.LoginWithTotpResponse], error) {
	ticket, err := s.repo.ConsumeOauthSignupTicket(
		ctx,
		auth.HashSessionToken(req.Msg.GetMfaTicket()),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("sign-in attempt expired, start over"),
		)
	}
	if err != nil {
		return nil, s.internal(ctx, "consume mfa ticket", err)
	}
	if ticket.Provider != mfaTicketProvider {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("sign-in attempt expired, start over"),
		)
	}
	userID, err := uuid.Parse(ticket.ProviderID)
	if err != nil {
		return nil, s.internal(ctx, "parse mfa ticket subject", err)
	}

	mfa, err := s.repo.GetUserMfa(ctx, userID)
	if err != nil || !mfa.ActivatedAt.Valid {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("sign-in attempt expired, start over"),
		)
	}

	code := strings.TrimSpace(req.Msg.GetCode())
	if !totp.Validate(code, mfa.TotpSecret) {
		sum := sha256.Sum256([]byte(code))
		rows, err := s.repo.ConsumeMfaRecoveryCode(ctx, repository.ConsumeMfaRecoveryCodeParams{
			UserID:   userID,
			CodeHash: sum[:],
		})
		if err != nil {
			return nil, s.internal(ctx, "consume recovery code", err)
		}
		if rows == 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid code"))
		}
	}

	token, identity, err := s.createSession(ctx, userID, deviceInfo(req.Header(), req.Peer().Addr))
	if err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&authv1.LoginWithTotpResponse{
		User:         s.currentUser(ctx, identity),
		SessionToken: token,
	})
	resp.Header().Add("Set-Cookie", s.sessionCookie(token, s.policy.TTL).String())
	return resp, nil
}

// mfaTicketProvider namespaces MFA tickets inside oauth_signup_tickets — same
// lifecycle (hash-only, single-use, TTL, janitor) so no extra table.
const mfaTicketProvider = "_mfa"

func (s *AuthService) issueMfaTicket(ctx context.Context, userID uuid.UUID) (string, error) {
	ticket, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("generate mfa ticket: %w", err)
	}
	if _, err := s.repo.CreateOauthSignupTicket(ctx, repository.CreateOauthSignupTicketParams{
		Provider:   mfaTicketProvider,
		ProviderID: userID.String(),
		SecretHash: auth.HashSessionToken(ticket),
		ExpiresAt:  time.Now().Add(mfaTicketTTL),
	}); err != nil {
		return "", fmt.Errorf("store mfa ticket: %w", err)
	}
	return ticket, nil
}

func randomRecoveryCode() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(base64.RawURLEncoding.EncodeToString(buf)), nil
}
