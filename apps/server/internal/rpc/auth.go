package rpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imbytecat/moonbase/packages/integrations/captcha"
	mail "github.com/imbytecat/moonbase/packages/integrations/email"
	"github.com/imbytecat/moonbase/packages/integrations/oauth"
	"github.com/imbytecat/moonbase/packages/integrations/sms"
	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/verify"
)

var errInvalidCredentials = connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))

type AuthService struct {
	repo         repository.Querier
	settings     *settings.Store
	captcha      captcha.Verifier
	mailer       mail.Sender
	smser        sms.Sender
	oauth        oauth.Flow
	verifier     *verify.Service
	logger       *slog.Logger
	policy       auth.SessionPolicy
	secureCookie bool
	publicURL    string
}

type AuthServiceDeps struct {
	Repo     repository.Querier
	Settings *settings.Store
	Captcha  captcha.Verifier
	Mailer   mail.Sender
	Smser    sms.Sender
	Oauth    oauth.Flow
	Verifier *verify.Service
	Logger   *slog.Logger
	Policy   auth.SessionPolicy
	// SecureCookie sets the cookie Secure flag (and the __Host- name).
	SecureCookie bool
	// PublicURL is the origin used in emailed links, e.g. https://app.example.com.
	PublicURL string
}

func NewAuthService(d AuthServiceDeps) *AuthService {
	return &AuthService{
		repo:         d.Repo,
		settings:     d.Settings,
		captcha:      d.Captcha,
		mailer:       d.Mailer,
		smser:        d.Smser,
		oauth:        d.Oauth,
		verifier:     d.Verifier,
		logger:       d.Logger,
		policy:       d.Policy,
		secureCookie: d.SecureCookie,
		publicURL:    strings.TrimSuffix(d.PublicURL, "/"),
	}
}

var _ authv1connect.AuthServiceHandler = (*AuthService)(nil)

func (s *AuthService) Login(
	ctx context.Context,
	req *connect.Request[authv1.LoginRequest],
) (*connect.Response[authv1.LoginResponse], error) {
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	user, err := s.userByIdentifier(ctx, req.Msg.GetIdentifier())
	if errors.Is(err, pgx.ErrNoRows) {
		// Burn an argon2 verification anyway: without it, "unknown identifier"
		// returns measurably faster than "wrong password", letting an
		// attacker probe which accounts exist.
		_, _ = auth.VerifyPassword(req.Msg.GetPassword(), auth.DummyHash)
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, s.internal(ctx, "get user by identifier", err)
	}
	ok, err := auth.VerifyPassword(req.Msg.GetPassword(), user.PasswordHash)
	if err != nil {
		return nil, s.internal(ctx, "verify password", err)
	}
	if !ok || !user.IsActive {
		return nil, errInvalidCredentials
	}

	// A confirmed TOTP factor turns login into two steps: correct password
	// yields only a short-lived ticket, never a session.
	if mfa, err := s.repo.GetUserMfa(ctx, user.ID); err == nil && mfa.ActivatedAt.Valid {
		ticket, err := s.issueMfaTicket(ctx, user.ID)
		if err != nil {
			return nil, s.internal(ctx, "issue mfa ticket", err)
		}
		return connect.NewResponse(&authv1.LoginResponse{
			MfaRequired: true,
			MfaTicket:   ticket,
		}), nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.internal(ctx, "load mfa", err)
	}

	token, identity, err := s.createSession(ctx, user.ID, deviceInfo(req.Header(), req.Peer().Addr))
	if err != nil {
		return nil, err
	}

	resp := connect.NewResponse(&authv1.LoginResponse{
		User:         s.currentUser(ctx, identity),
		SessionToken: token,
	})
	resp.Header().Add("Set-Cookie", s.sessionCookie(token, s.policy.TTL).String())
	return resp, nil
}

// userByIdentifier routes a login identifier by shape: "@" = email, "+" or
// digits = phone (E.164-normalized), anything else = username. Usernames must
// start with a letter, so the three shapes are disjoint and the routing is
// deterministic — no account can shadow another via a different identifier kind.
func (s *AuthService) userByIdentifier(ctx context.Context, identifier string) (repository.User, error) {
	switch {
	case strings.Contains(identifier, "@"):
		return s.repo.GetUserByEmail(ctx, identifier)
	case looksLikePhone(identifier):
		e164, _, err := phone.NormalizeWithRegion(identifier, s.phoneRegionHint(ctx))
		if err != nil {
			return repository.User{}, pgx.ErrNoRows
		}
		return s.repo.GetUserByPhone(ctx, e164)
	default:
		return s.repo.GetUserByUsername(ctx, identifier)
	}
}

func looksLikePhone(identifier string) bool {
	rest := strings.TrimPrefix(identifier, "+")
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if (r < '0' || r > '9') && r != ' ' && r != '-' {
			return false
		}
	}
	return true
}

// createSession is the shared post-authentication step for every login method
// (password, SMS): new token, session row, fully-resolved identity.
func (s *AuthService) createSession(ctx context.Context, userID uuid.UUID, device sessionDevice) (string, *auth.Identity, error) {
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return "", nil, s.internal(ctx, "create session token", err)
	}
	if _, err := s.repo.CreateSession(ctx, repository.CreateSessionParams{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: s.policy.InitialExpiry(time.Now()),
		UserAgent: device.userAgent,
		Ip:        device.ip,
	}); err != nil {
		return "", nil, s.internal(ctx, "create session", err)
	}
	row, err := s.repo.GetSessionIdentity(ctx, hash)
	if err != nil {
		return "", nil, s.internal(ctx, "load session identity", err)
	}
	return token, identityFromRow(row), nil
}

type sessionDevice struct {
	userAgent string
	ip        string
}

func deviceInfo(h http.Header, remoteAddr string) sessionDevice {
	ip, _, _ := net.SplitHostPort(remoteAddr)
	if ip == "" {
		ip = remoteAddr
	}
	ua := h.Get("User-Agent")
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return sessionDevice{userAgent: ua, ip: ip}
}

func identityFromRow(row repository.GetSessionIdentityRow) *auth.Identity {
	return &auth.Identity{
		UserID:        row.UserID,
		SessionID:     row.SessionID,
		Username:      row.Username,
		Email:         row.Email,
		Name:          row.Name,
		AvatarFileID:  row.AvatarFileID,
		Phone:         row.Phone,
		EmailVerified: row.EmailVerified,
		Permissions:   auth.PermissionSet(row.Permissions...),
	}
}

func (s *AuthService) Logout(
	ctx context.Context,
	req *connect.Request[authv1.LogoutRequest],
) (*connect.Response[authv1.LogoutResponse], error) {
	// Works for cookie and Bearer callers alike: the middleware already
	// resolved the session, so revoke by id; fall back to the raw cookie for
	// tokens the middleware no longer recognizes (expired but still held).
	if id := auth.IdentityFromContext(ctx); id != nil {
		if err := s.repo.DeleteSession(ctx, id.SessionID); err != nil {
			s.logger.ErrorContext(ctx, "delete session failed", "error", err)
		}
	} else if cookie := sessionTokenFromRequest(req.Header()); cookie != "" {
		if err := s.repo.DeleteSessionByTokenHash(ctx, auth.HashSessionToken(cookie)); err != nil {
			s.logger.ErrorContext(ctx, "delete session failed", "error", err)
		}
	}
	resp := connect.NewResponse(&authv1.LogoutResponse{})
	resp.Header().Add("Set-Cookie", s.sessionCookie("", -time.Hour).String())
	return resp, nil
}

func (s *AuthService) GetMe(
	ctx context.Context,
	_ *connect.Request[authv1.GetMeRequest],
) (*connect.Response[authv1.GetMeResponse], error) {
	id := auth.IdentityFromContext(ctx)
	return connect.NewResponse(&authv1.GetMeResponse{User: s.currentUser(ctx, id)}), nil
}

func (s *AuthService) Register(
	ctx context.Context,
	req *connect.Request[authv1.RegisterRequest],
) (*connect.Response[authv1.RegisterResponse], error) {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load auth settings", err)
	}
	if !authCfg.RegistrationEnabled {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("registration is disabled"))
	}
	if err := s.verifyCaptcha(ctx, req.Msg.GetCaptchaToken(), req.Peer().Addr); err != nil {
		return nil, err
	}
	values, err := signupIdentifierValues(authCfg, req.Msg)
	if err != nil {
		return nil, err
	}

	// Channel-backed identifiers must prove ownership before the row exists:
	// consuming the code binds the identifier to whoever holds the secret.
	if values.phone != "" {
		e164, err := s.normalizedAllowedPhone(ctx, values.phone)
		if err != nil {
			return nil, err
		}
		values.phone = e164
		if _, err := s.verifier.ConsumeCode(ctx, verify.PurposePhoneRegister, e164, req.Msg.GetPhoneCode()); err != nil {
			return nil, s.verifyConsumeError(ctx, "consume phone register code", err)
		}
	}
	if values.email != "" {
		normalized := strings.ToLower(values.email)
		if _, err := s.verifier.ConsumeCode(ctx, verify.PurposeEmailRegister, normalized, req.Msg.GetEmailCode()); err != nil {
			return nil, s.verifyConsumeError(ctx, "consume email register code", err)
		}
		values.emailVerified = true
	}

	hash, err := auth.HashPassword(req.Msg.GetPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	user, err := s.repo.CreateUser(ctx, repository.CreateUserParams{
		Username:      values.username,
		Email:         values.email,
		Name:          req.Msg.GetName(),
		PasswordHash:  hash,
		Phone:         values.phone,
		EmailVerified: values.emailVerified,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("this account is already registered"))
		}
		return nil, s.internal(ctx, "create user", err)
	}

	role, err := s.repo.GetRoleByName(ctx, "user")
	if err == nil {
		if err := s.repo.AddUserRoles(ctx, repository.AddUserRolesParams{
			UserID:  user.ID,
			RoleIds: []uuid.UUID{role.ID},
		}); err != nil {
			return nil, s.internal(ctx, "assign default role", err)
		}
	}

	return connect.NewResponse(&authv1.RegisterResponse{User: &authv1.CurrentUser{
		Id:       user.ID.String(),
		Username: user.Username,
		Email:    user.Email,
		Name:     user.Name,
		Phone:    user.Phone,
	}}), nil
}

type signupValues struct {
	username      string
	email         string
	phone         string
	emailVerified bool
}

// signupIdentifierValues enforces the business policy the register form was
// rendered from: identifiers in the policy are required, ones outside it are
// rejected (a client must not sneak in fields the deployment chose not to
// collect).
func signupIdentifierValues(cfg settings.Auth, msg *authv1.RegisterRequest) (signupValues, error) {
	invalid := func(text string) (signupValues, error) {
		return signupValues{}, connect.NewError(connect.CodeInvalidArgument, errors.New(text))
	}
	required := make(map[string]bool, 3)
	for _, id := range cfg.EffectiveSignupIdentifiers() {
		required[id] = true
	}
	if required[settings.SignupIdentifierUsername] != (msg.GetUsername() != "") {
		if required[settings.SignupIdentifierUsername] {
			return invalid("username is required")
		}
		return invalid("username is not accepted")
	}
	if required[settings.SignupIdentifierEmail] != (msg.GetEmail() != "") {
		if required[settings.SignupIdentifierEmail] {
			return invalid("email is required")
		}
		return invalid("email is not accepted")
	}
	if required[settings.SignupIdentifierPhone] != (msg.GetPhone() != "") {
		if required[settings.SignupIdentifierPhone] {
			return invalid("phone is required")
		}
		return invalid("phone is not accepted")
	}
	if msg.GetPhone() != "" && msg.GetPhoneCode() == "" {
		return invalid("phone verification code is required")
	}
	if msg.GetEmail() != "" && msg.GetEmailCode() == "" {
		return invalid("email verification code is required")
	}
	return signupValues{username: msg.GetUsername(), email: msg.GetEmail(), phone: msg.GetPhone()}, nil
}

func (s *AuthService) GetAuthConfig(
	ctx context.Context,
	_ *connect.Request[authv1.GetAuthConfigRequest],
) (*connect.Response[authv1.GetAuthConfigResponse], error) {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load auth settings", err)
	}
	out := &authv1.GetAuthConfigResponse{
		RegistrationEnabled: authCfg.RegistrationEnabled,
		AllowedPhoneRegions: authCfg.AllowedPhoneRegions,
		SignupIdentifiers:   authCfg.EffectiveSignupIdentifiers(),
	}
	capCfg, err := s.settings.Captcha(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load captcha settings", err)
	}
	if provider, siteKey, ok := captcha.Widget(capCfg, captcha.PurposeAuth); ok {
		out.CaptchaProvider = provider
		out.CaptchaSiteKey = siteKey
	}
	emailCfg, err := s.settings.Email(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load email settings", err)
	}
	out.EmailEnabled = mail.Usable(emailCfg, mail.PurposeAuth)
	smsCfg, err := s.settings.Sms(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load sms settings", err)
	}
	out.SmsEnabled = sms.Usable(smsCfg, sms.PurposeVerification)
	oauthCfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	for _, opt := range oauth.UsableProviders(oauthCfg) {
		out.OauthProviders = append(out.OauthProviders, &authv1.OauthProviderOption{
			Key:      opt.Key,
			Name:     opt.Name,
			Provider: opt.Provider,
		})
	}
	return connect.NewResponse(out), nil
}

// verifyCaptcha maps integration state to RPC errors: unbound purpose = pass,
// bad/missing token = invalid_argument (fail closed once the purpose is bound).
func (s *AuthService) verifyCaptcha(ctx context.Context, token, remoteAddr string) error {
	ip, _, _ := net.SplitHostPort(remoteAddr)
	if err := s.captcha.Verify(ctx, captcha.PurposeAuth, token, ip); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("captcha verification failed"))
	}
	return nil
}

func (s *AuthService) UpdateProfile(
	ctx context.Context,
	req *connect.Request[authv1.UpdateProfileRequest],
) (*connect.Response[authv1.UpdateProfileResponse], error) {
	id := auth.IdentityFromContext(ctx)
	user, err := s.repo.UpdateUser(ctx, repository.UpdateUserParams{
		ID:     id.UserID,
		Name:   textArg(req.Msg.Name),
		Locale: textArg(req.Msg.Locale),
	})
	if err != nil {
		return nil, s.internal(ctx, "update profile", err)
	}
	updated := *id
	updated.Name = user.Name
	updated.Locale = user.Locale
	// The avatar is a file reference, not a user-row column: only touch it when
	// the client sends the field, transferring the attachment to the new file.
	if req.Msg.AvatarFileId != nil {
		if err := s.setAvatar(ctx, id.UserID, req.Msg.GetAvatarFileId()); err != nil {
			return nil, err
		}
		updated.AvatarFileID = req.Msg.GetAvatarFileId()
	}
	return connect.NewResponse(&authv1.UpdateProfileResponse{User: s.currentUser(ctx, &updated)}), nil
}

// setAvatar points the caller's avatar slot at fileID (empty clears it). It
// refuses any file the caller did not upload as an avatar, so no one can
// attach another user's file to themselves.
func (s *AuthService) setAvatar(ctx context.Context, userID uuid.UUID, fileID string) error {
	var param pgtype.UUID
	if fileID != "" {
		parsed, err := uuid.Parse(fileID)
		if err != nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid avatar file id"))
		}
		file, err := s.repo.GetFile(ctx, parsed)
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("unknown avatar file"))
		}
		if err != nil {
			return s.internal(ctx, "get avatar file", err)
		}
		if file.UploadedBy != userID || file.Purpose != storage.PurposeAvatars {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("file is not an avatar you uploaded"))
		}
		param = pgtype.UUID{Bytes: parsed, Valid: true}
	}
	if err := s.repo.SetUserAvatar(ctx, repository.SetUserAvatarParams{
		UserID: userID,
		FileID: param,
	}); err != nil {
		return s.internal(ctx, "set avatar", err)
	}
	return nil
}

func (s *AuthService) ChangePassword(
	ctx context.Context,
	req *connect.Request[authv1.ChangePasswordRequest],
) (*connect.Response[authv1.ChangePasswordResponse], error) {
	id := auth.IdentityFromContext(ctx)
	user, err := s.repo.GetUser(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "get user", err)
	}
	ok, err := auth.VerifyPassword(req.Msg.GetCurrentPassword(), user.PasswordHash)
	if err != nil {
		return nil, s.internal(ctx, "verify password", err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("current password is incorrect"))
	}
	hash, err := auth.HashPassword(req.Msg.GetNewPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	if err := s.repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
		ID:           id.UserID,
		PasswordHash: hash,
	}); err != nil {
		return nil, s.internal(ctx, "update password", err)
	}
	// Changing the password revokes every other session so a stolen session
	// can't outlive a password reset. The current session stays valid.
	if err := s.repo.DeleteOtherUserSessions(ctx, repository.DeleteOtherUserSessionsParams{
		UserID: id.UserID,
		ID:     id.SessionID,
	}); err != nil {
		return nil, s.internal(ctx, "revoke other sessions", err)
	}
	return connect.NewResponse(&authv1.ChangePasswordResponse{}), nil
}

// currentUser shapes an Identity for the wire, resolving the avatar key to a
// fetchable URL (best-effort: storage problems just mean no URL).
func (s *AuthService) currentUser(ctx context.Context, id *auth.Identity) *authv1.CurrentUser {
	perms := make([]authv1.Permission, 0, len(id.Permissions))
	for p := range id.Permissions {
		perms = append(perms, permissionEnum(p))
	}
	slices.Sort(perms)
	out := &authv1.CurrentUser{
		Id:            id.UserID.String(),
		Username:      id.Username,
		Email:         id.Email,
		Name:          id.Name,
		Phone:         id.Phone,
		EmailVerified: id.EmailVerified,
		Permissions:   perms,
		Locale:        id.Locale,
	}
	if id.AvatarFileID != "" {
		out.AvatarUrl = "/f/" + id.AvatarFileID
	}
	if mfa, err := s.repo.GetUserMfa(ctx, id.UserID); err == nil && mfa.ActivatedAt.Valid {
		out.TotpEnabled = true
	}
	return out
}

func (s *AuthService) sessionCookie(token string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     auth.CookieName(s.secureCookie),
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *AuthService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func sessionTokenFromRequest(h http.Header) string {
	req := http.Request{Header: h}
	for _, name := range [2]string{auth.CookieName(true), auth.CookieName(false)} {
		if cookie, err := req.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

func textArg(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
