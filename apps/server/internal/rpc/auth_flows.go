package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	mail "github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/verify"
)

const (
	emailVerifySubject = "验证你的邮箱"
	emailVerifyBody    = "你好 %s，\n\n点击下方链接验证你的邮箱地址，链接 24 小时内有效。\n\n%s\n"
	emailResetSubject  = "重置你的密码"
	emailResetBody     = "你好 %s，\n\n点击下方链接设置新密码，链接 1 小时内有效。如果这不是你本人操作，请忽略此邮件。\n\n%s\n"
	emailCodeSubject   = "你的验证码"
	emailCodeBody      = "你的验证码是 %s，5 分钟内有效。\n"
	emailCodeBodyNamed = "你好 %s，\n\n你的验证码是 %s，5 分钟内有效。\n"
)

var errChannelUnavailable = connect.NewError(connect.CodeFailedPrecondition,
	errors.New("this feature is not available"))

func (s *AuthService) SendVerificationEmail(
	ctx context.Context,
	req *connect.Request[authv1.SendVerificationEmailRequest],
) (*connect.Response[authv1.SendVerificationEmailResponse], error) {
	id := auth.IdentityFromContext(ctx)
	user, err := s.repo.GetUser(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "get user", err)
	}
	if user.EmailVerifiedAt.Valid {
		return connect.NewResponse(&authv1.SendVerificationEmailResponse{}), nil
	}

	token, err := s.verifier.IssueLinkToken(ctx, verify.PurposeEmailVerify, user.Email, user.ID)
	if err != nil {
		return nil, s.verifyIssueError(ctx, "issue email verify token", err)
	}
	link := s.publicURL + "/verify-email?token=" + url.QueryEscape(token)
	if err := s.mailer.Send(ctx, mail.PurposeAuth, user.Email, emailVerifySubject,
		fmt.Sprintf(emailVerifyBody, user.Name, link)); err != nil {
		if errors.Is(err, mail.ErrNotConfigured) {
			return nil, errChannelUnavailable
		}
		return nil, s.internal(ctx, "send verification email", err)
	}
	return connect.NewResponse(&authv1.SendVerificationEmailResponse{}), nil
}

func (s *AuthService) VerifyEmail(
	ctx context.Context,
	req *connect.Request[authv1.VerifyEmailRequest],
) (*connect.Response[authv1.VerifyEmailResponse], error) {
	row, err := s.verifier.ConsumeLinkToken(ctx, verify.PurposeEmailVerify, req.Msg.GetToken())
	if err != nil {
		return nil, s.verifyConsumeError(ctx, "verify email", err)
	}
	if !row.UserID.Valid {
		return nil, s.internal(ctx, "verify email", errors.New("token has no user"))
	}
	if err := s.repo.SetUserEmailVerified(ctx, row.UserID.Bytes); err != nil {
		return nil, s.internal(ctx, "mark email verified", err)
	}
	return connect.NewResponse(&authv1.VerifyEmailResponse{}), nil
}

func (s *AuthService) RequestPasswordReset(
	ctx context.Context,
	req *connect.Request[authv1.RequestPasswordResetRequest],
) (*connect.Response[authv1.RequestPasswordResetResponse], error) {
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	// Anti-enumeration: this RPC answers ok no matter what. Failures after
	// this point are logged, never surfaced.
	resp := connect.NewResponse(&authv1.RequestPasswordResetResponse{})

	emailUsable, err := s.mailer.Usable(ctx, mail.PurposeAuth)
	if err != nil || !emailUsable {
		return resp, nil
	}
	user, err := s.repo.GetUserByEmail(ctx, req.Msg.GetEmail())
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !user.IsActive) {
		return resp, nil
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "password reset lookup failed", "error", err)
		return resp, nil
	}

	token, err := s.verifier.IssueLinkToken(ctx, verify.PurposePasswordReset, user.Email, user.ID)
	if err != nil {
		if !errors.Is(err, verify.ErrRateLimited) {
			s.logger.ErrorContext(ctx, "issue reset token failed", "error", err)
		}
		return resp, nil
	}
	link := s.publicURL + "/reset-password?token=" + url.QueryEscape(token)
	if err := s.mailer.Send(ctx, mail.PurposeAuth, user.Email, emailResetSubject,
		fmt.Sprintf(emailResetBody, user.Name, link)); err != nil {
		s.logger.ErrorContext(ctx, "send reset email failed", "error", err)
	}
	return resp, nil
}

func (s *AuthService) ResetPassword(
	ctx context.Context,
	req *connect.Request[authv1.ResetPasswordRequest],
) (*connect.Response[authv1.ResetPasswordResponse], error) {
	row, err := s.verifier.ConsumeLinkToken(ctx, verify.PurposePasswordReset, req.Msg.GetToken())
	if err != nil {
		return nil, s.verifyConsumeError(ctx, "reset password", err)
	}
	if !row.UserID.Valid {
		return nil, s.internal(ctx, "reset password", errors.New("token has no user"))
	}
	userID := uuid.UUID(row.UserID.Bytes)

	hash, err := auth.HashPassword(req.Msg.GetNewPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	if err := s.repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
		ID:           userID,
		PasswordHash: hash,
	}); err != nil {
		return nil, s.internal(ctx, "update password", err)
	}
	// A reset proves channel ownership, not session ownership: kill every
	// session (the attacker may be the one holding a live session).
	if err := s.repo.DeleteUserSessions(ctx, userID); err != nil {
		return nil, s.internal(ctx, "revoke sessions", err)
	}
	return connect.NewResponse(&authv1.ResetPasswordResponse{}), nil
}

func (s *AuthService) SendPhoneBindCode(
	ctx context.Context,
	req *connect.Request[authv1.SendPhoneBindCodeRequest],
) (*connect.Response[authv1.SendPhoneBindCodeResponse], error) {
	id := auth.IdentityFromContext(ctx)
	e164, err := s.normalizedAllowedPhone(ctx, req.Msg.GetPhoneNumber())
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.GetUserByPhone(ctx, e164); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			errors.New("this phone number is already bound to an account"))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.internal(ctx, "check phone", err)
	}

	code, err := s.verifier.IssueCode(ctx, verify.PurposePhoneBind, e164, id.UserID)
	if err != nil {
		return nil, s.verifyIssueError(ctx, "issue bind code", err)
	}
	if err := s.smser.SendCode(ctx, sms.PurposeVerification, e164, code); err != nil {
		if errors.Is(err, sms.ErrNotConfigured) {
			return nil, errChannelUnavailable
		}
		return nil, s.internal(ctx, "send bind code", err)
	}
	return connect.NewResponse(&authv1.SendPhoneBindCodeResponse{}), nil
}

func (s *AuthService) BindPhone(
	ctx context.Context,
	req *connect.Request[authv1.BindPhoneRequest],
) (*connect.Response[authv1.BindPhoneResponse], error) {
	id := auth.IdentityFromContext(ctx)
	e164, err := s.normalizedAllowedPhone(ctx, req.Msg.GetPhoneNumber())
	if err != nil {
		return nil, err
	}

	row, err := s.verifier.ConsumeCode(ctx, verify.PurposePhoneBind, e164, req.Msg.GetCode())
	if err != nil {
		return nil, s.verifyConsumeError(ctx, "bind phone", err)
	}
	// The code must have been issued for THIS user binding THIS phone.
	if !row.UserID.Valid || uuid.UUID(row.UserID.Bytes) != id.UserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid or expired code"))
	}
	if err := s.repo.SetUserPhone(ctx, repository.SetUserPhoneParams{
		ID:    id.UserID,
		Phone: e164,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				errors.New("this phone number is already bound to an account"))
		}
		return nil, s.internal(ctx, "bind phone", err)
	}
	updated := *id
	return connect.NewResponse(&authv1.BindPhoneResponse{
		User: s.currentUser(ctx, &updated),
	}), nil
}

func (s *AuthService) SendSmsLoginCode(
	ctx context.Context,
	req *connect.Request[authv1.SendSmsLoginCodeRequest],
) (*connect.Response[authv1.SendSmsLoginCodeResponse], error) {
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	// Anti-enumeration: always ok; a code only goes to bound, active phones.
	// Region policy failures are also silent here (a bound number that later
	// fell outside a tightened policy just stops receiving codes).
	resp := connect.NewResponse(&authv1.SendSmsLoginCodeResponse{})
	e164, _, err := phone.NormalizeWithRegion(req.Msg.GetPhoneNumber(), s.phoneRegionHint(ctx))
	if err != nil {
		return resp, nil
	}

	user, err := s.repo.GetUserByPhone(ctx, e164)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !user.IsActive) {
		return resp, nil
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "sms login lookup failed", "error", err)
		return resp, nil
	}

	code, err := s.verifier.IssueCode(ctx, verify.PurposeSmsLogin, e164, user.ID)
	if err != nil {
		if !errors.Is(err, verify.ErrRateLimited) {
			s.logger.ErrorContext(ctx, "issue login code failed", "error", err)
		}
		return resp, nil
	}
	if err := s.smser.SendCode(ctx, sms.PurposeVerification, e164, code); err != nil {
		s.logger.ErrorContext(ctx, "send login code failed", "error", err)
	}
	return resp, nil
}

func (s *AuthService) LoginWithSms(
	ctx context.Context,
	req *connect.Request[authv1.LoginWithSmsRequest],
) (*connect.Response[authv1.LoginWithSmsResponse], error) {
	e164, _, err := phone.NormalizeWithRegion(req.Msg.GetPhoneNumber(), s.phoneRegionHint(ctx))
	if err != nil {
		return nil, errInvalidCredentials
	}
	row, err := s.verifier.ConsumeCode(ctx, verify.PurposeSmsLogin, e164, req.Msg.GetCode())
	if err != nil {
		return nil, s.verifyConsumeError(ctx, "sms login", err)
	}
	user, err := s.repo.GetUserByPhone(ctx, e164)
	if err != nil || !user.IsActive || !row.UserID.Valid || uuid.UUID(row.UserID.Bytes) != user.ID {
		return nil, errInvalidCredentials
	}

	token, identity, err := s.createSession(ctx, user.ID, deviceInfo(req.Header(), req.Peer().Addr))
	if err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&authv1.LoginWithSmsResponse{
		User:         s.currentUser(ctx, identity),
		SessionToken: token,
	})
	resp.Header().Add("Set-Cookie", s.sessionCookie(token, s.policy.TTL).String())
	return resp, nil
}

// normalizedAllowedPhone parses to E.164 and enforces the region policy —
// the single gate every user-facing phone input passes through. A single-region
// policy also lets bare national numbers through (country assumed).
func (s *AuthService) normalizedAllowedPhone(ctx context.Context, input string) (string, error) {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return "", s.internal(ctx, "load auth settings", err)
	}
	e164, region, err := phone.NormalizeWithRegion(input, phone.DefaultRegion(authCfg.AllowedPhoneRegions))
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, phone.ErrInvalid)
	}
	if err := phone.Allowed(region, authCfg.AllowedPhoneRegions); err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, phone.ErrRegionNotAllowed)
	}
	return e164, nil
}

// phoneRegionHint is the country to assume for bare national numbers typed into
// freeform phone fields (the login identifier, the SMS-code phone): the sole
// allowed region when the policy pins exactly one country, else "" (require
// E.164). A settings-read failure degrades to "" rather than blocking login.
func (s *AuthService) phoneRegionHint(ctx context.Context) string {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "load auth settings for phone region hint", "error", err)
		return ""
	}
	return phone.DefaultRegion(authCfg.AllowedPhoneRegions)
}

// SendPhoneRegisterCode proves ownership of a phone BEFORE the account row
// exists (uuid.Nil user). Taken phones answer already_exists: a signup form
// legitimately reveals taken identifiers (unique violation shows it anyway),
// unlike login-side flows which stay silent.
func (s *AuthService) SendPhoneRegisterCode(
	ctx context.Context,
	req *connect.Request[authv1.SendPhoneRegisterCodeRequest],
) (*connect.Response[authv1.SendPhoneRegisterCodeResponse], error) {
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	e164, err := s.normalizedAllowedPhone(ctx, req.Msg.GetPhoneNumber())
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetUserByPhone(ctx, e164); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			errors.New("this phone number is already registered"))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.internal(ctx, "check phone", err)
	}
	code, err := s.verifier.IssueCode(ctx, verify.PurposePhoneRegister, e164, uuid.Nil)
	if err != nil {
		return nil, s.verifyIssueError(ctx, "issue phone register code", err)
	}
	if err := s.smser.SendCode(ctx, sms.PurposeVerification, e164, code); err != nil {
		if errors.Is(err, sms.ErrNotConfigured) {
			return nil, errChannelUnavailable
		}
		return nil, s.internal(ctx, "send phone register code", err)
	}
	return connect.NewResponse(&authv1.SendPhoneRegisterCodeResponse{}), nil
}

// SendEmailRegisterCode is the email counterpart, used when the signup
// policy collects email.
func (s *AuthService) SendEmailRegisterCode(
	ctx context.Context,
	req *connect.Request[authv1.SendEmailRegisterCodeRequest],
) (*connect.Response[authv1.SendEmailRegisterCodeResponse], error) {
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	email := strings.ToLower(req.Msg.GetEmail())
	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			errors.New("this email is already registered"))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.internal(ctx, "check email", err)
	}
	code, err := s.verifier.IssueCode(ctx, verify.PurposeEmailRegister, email, uuid.Nil)
	if err != nil {
		return nil, s.verifyIssueError(ctx, "issue email register code", err)
	}
	if err := s.mailer.Send(ctx, mail.PurposeAuth, email, emailCodeSubject,
		fmt.Sprintf(emailCodeBody, code)); err != nil {
		if errors.Is(err, mail.ErrNotConfigured) {
			return nil, errChannelUnavailable
		}
		return nil, s.internal(ctx, "send email register code", err)
	}
	return connect.NewResponse(&authv1.SendEmailRegisterCodeResponse{}), nil
}

// SendEmailBindCode is the email counterpart of SendPhoneBindCode: a code to
// attach an email to the CURRENT user's account.
func (s *AuthService) SendEmailBindCode(
	ctx context.Context,
	req *connect.Request[authv1.SendEmailBindCodeRequest],
) (*connect.Response[authv1.SendEmailBindCodeResponse], error) {
	id := auth.IdentityFromContext(ctx)
	email := strings.ToLower(req.Msg.GetEmail())
	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			errors.New("this email is already bound to an account"))
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.internal(ctx, "check email", err)
	}
	code, err := s.verifier.IssueCode(ctx, verify.PurposeEmailBind, email, id.UserID)
	if err != nil {
		return nil, s.verifyIssueError(ctx, "issue email bind code", err)
	}
	if err := s.mailer.Send(ctx, mail.PurposeAuth, email, emailCodeSubject,
		fmt.Sprintf(emailCodeBodyNamed, id.Name, code)); err != nil {
		if errors.Is(err, mail.ErrNotConfigured) {
			return nil, errChannelUnavailable
		}
		return nil, s.internal(ctx, "send email bind code", err)
	}
	return connect.NewResponse(&authv1.SendEmailBindCodeResponse{}), nil
}

// BindEmail consumes the code and stores the email as verified — the code
// round-trip IS the ownership proof.
func (s *AuthService) BindEmail(
	ctx context.Context,
	req *connect.Request[authv1.BindEmailRequest],
) (*connect.Response[authv1.BindEmailResponse], error) {
	id := auth.IdentityFromContext(ctx)
	email := strings.ToLower(req.Msg.GetEmail())
	row, err := s.verifier.ConsumeCode(ctx, verify.PurposeEmailBind, email, req.Msg.GetCode())
	if err != nil {
		return nil, s.verifyConsumeError(ctx, "bind email", err)
	}
	if !row.UserID.Valid || uuid.UUID(row.UserID.Bytes) != id.UserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid or expired code"))
	}
	if err := s.repo.SetUserEmail(ctx, repository.SetUserEmailParams{
		ID:    id.UserID,
		Email: email,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				errors.New("this email is already bound to an account"))
		}
		return nil, s.internal(ctx, "bind email", err)
	}
	updated := *id
	updated.Email = email
	updated.EmailVerified = true
	return connect.NewResponse(&authv1.BindEmailResponse{
		User: s.currentUser(ctx, &updated),
	}), nil
}

// requirePassword gates identifier-lifecycle actions: a hijacked cookie alone
// must not be able to strip recovery channels.
func (s *AuthService) requirePassword(ctx context.Context, userID uuid.UUID, password string) error {
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return s.internal(ctx, "get user", err)
	}
	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return s.internal(ctx, "verify password", err)
	}
	if !ok {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("current password is incorrect"))
	}
	return nil
}

// UnbindPhone clears the phone; the guarded UPDATE refuses to strip the last
// remaining identifier (WHERE username <> ” OR email <> ”).
func (s *AuthService) UnbindPhone(
	ctx context.Context,
	req *connect.Request[authv1.UnbindPhoneRequest],
) (*connect.Response[authv1.UnbindPhoneResponse], error) {
	id := auth.IdentityFromContext(ctx)
	if err := s.requirePassword(ctx, id.UserID, req.Msg.GetCurrentPassword()); err != nil {
		return nil, err
	}
	rows, err := s.repo.ClearUserPhone(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "unbind phone", err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("cannot remove the last login identifier"))
	}
	updated := *id
	updated.Phone = ""
	return connect.NewResponse(&authv1.UnbindPhoneResponse{User: s.currentUser(ctx, &updated)}), nil
}

func (s *AuthService) UnbindEmail(
	ctx context.Context,
	req *connect.Request[authv1.UnbindEmailRequest],
) (*connect.Response[authv1.UnbindEmailResponse], error) {
	id := auth.IdentityFromContext(ctx)
	if err := s.requirePassword(ctx, id.UserID, req.Msg.GetCurrentPassword()); err != nil {
		return nil, err
	}
	rows, err := s.repo.ClearUserEmail(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "unbind email", err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("cannot remove the last login identifier"))
	}
	updated := *id
	updated.Email = ""
	updated.EmailVerified = false
	return connect.NewResponse(&authv1.UnbindEmailResponse{User: s.currentUser(ctx, &updated)}), nil
}

func (s *AuthService) ListMySessions(
	ctx context.Context,
	_ *connect.Request[authv1.ListMySessionsRequest],
) (*connect.Response[authv1.ListMySessionsResponse], error) {
	id := auth.IdentityFromContext(ctx)
	rows, err := s.repo.ListUserSessions(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "list sessions", err)
	}
	out := make([]*authv1.Session, len(rows))
	for i, row := range rows {
		out[i] = &authv1.Session{
			Id:        row.ID.String(),
			UserAgent: row.UserAgent,
			Ip:        row.Ip,
			CreatedAt: timestamppb.New(row.CreatedAt),
			ExpiresAt: timestamppb.New(row.ExpiresAt),
			Current:   row.ID == id.SessionID,
		}
	}
	return connect.NewResponse(&authv1.ListMySessionsResponse{Sessions: out}), nil
}

// RevokeMySession deletes one of the CALLER's sessions — the user_id filter
// in the query makes revoking someone else's session impossible.
func (s *AuthService) RevokeMySession(
	ctx context.Context,
	req *connect.Request[authv1.RevokeMySessionRequest],
) (*connect.Response[authv1.RevokeMySessionResponse], error) {
	id := auth.IdentityFromContext(ctx)
	sessionID, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	rows, err := s.repo.DeleteUserSessionByID(ctx, repository.DeleteUserSessionByIDParams{
		ID:     sessionID,
		UserID: id.UserID,
	})
	if err != nil {
		return nil, s.internal(ctx, "revoke session", err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("session not found"))
	}
	return connect.NewResponse(&authv1.RevokeMySessionResponse{}), nil
}

// verifyIssueError maps issuance failures: rate limiting is the only caller
// mistake; everything else is internal.
func (s *AuthService) verifyIssueError(ctx context.Context, op string, err error) error {
	if errors.Is(err, verify.ErrRateLimited) {
		return connect.NewError(connect.CodeResourceExhausted, verify.ErrRateLimited)
	}
	return s.internal(ctx, op, err)
}

// verifyConsumeError keeps all invalid-secret cases indistinguishable.
func (s *AuthService) verifyConsumeError(ctx context.Context, op string, err error) error {
	if errors.Is(err, verify.ErrInvalid) {
		return connect.NewError(connect.CodeInvalidArgument, verify.ErrInvalid)
	}
	return s.internal(ctx, op, err)
}
