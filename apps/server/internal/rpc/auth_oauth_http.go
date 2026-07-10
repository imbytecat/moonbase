package rpc

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const (
	oauthStateCookie = "oauth_state"
	oauthStateTTL    = 10 * time.Minute
	signupTicketTTL  = 15 * time.Minute
)

// OauthAuthorize starts the browser flow: set a CSRF state cookie, redirect
// to the provider's authorization page. The cookie also carries the OIDC
// nonce and PKCE verifier minted for this flow (httpOnly, never exposed to
// the provider) so the callback can verify the ID token.
func (s *AuthService) OauthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	state, err := randomToken()
	if err != nil {
		s.oauthFail(w, r, "internal error")
		return
	}
	authorizeURL, secrets, err := s.oauth.AuthorizeURL(
		r.Context(),
		provider,
		s.oauthRedirectURI(provider),
		state,
	)
	if err != nil {
		s.oauthFail(w, r, "this login method is not available")
		return
	}
	cookieValue, err := encodeOauthState(
		oauthState{State: state, Nonce: secrets.Nonce, Verifier: secrets.Verifier},
	)
	if err != nil {
		s.oauthFail(w, r, "internal error")
		return
	}
	http.SetCookie(w, s.stateCookie(cookieValue, oauthStateTTL))
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// OauthCallback finishes the flow with three outcomes: sign in (identity
// known), bind (a session is active), or hand off to the completion page
// with a one-time signup ticket (new visitor).
func (s *AuthService) OauthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := r.PathValue("provider")

	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" {
		s.oauthFail(w, r, "sign-in attempt expired, please try again")
		return
	}
	flow, err := decodeOauthState(stateCookie.Value)
	if err != nil || flow.State == "" || flow.State != r.URL.Query().Get("state") {
		s.oauthFail(w, r, "sign-in attempt expired, please try again")
		return
	}
	http.SetCookie(w, s.stateCookie("", -time.Hour))
	code := r.URL.Query().Get("code")
	if code == "" {
		s.oauthFail(w, r, "sign-in was cancelled")
		return
	}

	external, err := s.oauth.Exchange(
		ctx,
		provider,
		code,
		s.oauthRedirectURI(provider),
		oauth.FlowSecrets{
			Nonce:    flow.Nonce,
			Verifier: flow.Verifier,
		},
	)
	if err != nil {
		s.logger.ErrorContext(ctx, "oauth exchange failed", "provider", provider, "error", err)
		s.oauthFail(w, r, "sign-in failed, please try again")
		return
	}

	identity, err := s.repo.GetIdentity(ctx, repository.GetIdentityParams{
		Provider:   external.ProviderKey,
		ProviderID: external.ProviderID,
	})
	switch {
	case err == nil:
		s.oauthSignIn(w, r, identity.UserID)
	case errors.Is(err, pgx.ErrNoRows):
		if caller := auth.IdentityFromContext(ctx); caller != nil {
			s.oauthBind(w, r, caller.UserID, external)
			return
		}
		s.oauthHandOff(w, r, external)
	default:
		s.logger.ErrorContext(ctx, "oauth identity lookup failed", "error", err)
		s.oauthFail(w, r, "sign-in failed, please try again")
	}
}

// oauthSignIn issues a session for the known identity — same cookie the
// password login sets. Disabled users get the generic failure.
func (s *AuthService) oauthSignIn(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	ctx := r.Context()
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil || !user.IsActive {
		s.oauthFail(w, r, "sign-in failed, please try again")
		return
	}
	token, _, err := s.createSession(ctx, userID, deviceInfo(r.Header, r.RemoteAddr))
	if err != nil {
		s.oauthFail(w, r, "sign-in failed, please try again")
		return
	}
	http.SetCookie(w, s.sessionCookie(token, s.policy.TTL))
	http.Redirect(w, r, "/", http.StatusFound)
}

// oauthBind links the external identity to the already-signed-in account and
// returns to the profile page.
func (s *AuthService) oauthBind(
	w http.ResponseWriter,
	r *http.Request,
	userID uuid.UUID,
	external oauth.ExternalIdentity,
) {
	ctx := r.Context()
	if _, err := s.repo.CreateIdentity(ctx, repository.CreateIdentityParams{
		UserID:     userID,
		Provider:   external.ProviderKey,
		ProviderID: external.ProviderID,
		Name:       external.Name,
		AvatarUrl:  external.AvatarURL,
	}); err != nil {
		if isUniqueViolation(err) {
			http.Redirect(w, r, "/profile?oauthError=already_bound", http.StatusFound)
			return
		}
		s.logger.ErrorContext(ctx, "oauth bind failed", "error", err)
		s.oauthFail(w, r, "binding failed, please try again")
		return
	}
	http.Redirect(
		w,
		r,
		"/profile?oauthBound="+url.QueryEscape(external.ProviderKey),
		http.StatusFound,
	)
}

// oauthHandOff stores a one-time signup ticket (only its hash) and sends the
// browser to the completion form; the RPC CompleteOauthSignup consumes it.
func (s *AuthService) oauthHandOff(
	w http.ResponseWriter,
	r *http.Request,
	external oauth.ExternalIdentity,
) {
	ctx := r.Context()
	ticket, err := randomToken()
	if err != nil {
		s.oauthFail(w, r, "internal error")
		return
	}
	if _, err := s.repo.CreateOauthSignupTicket(ctx, repository.CreateOauthSignupTicketParams{
		Provider:   external.ProviderKey,
		ProviderID: external.ProviderID,
		Name:       external.Name,
		AvatarUrl:  external.AvatarURL,
		SecretHash: auth.HashSessionToken(ticket),
		ExpiresAt:  time.Now().Add(signupTicketTTL),
	}); err != nil {
		s.logger.ErrorContext(ctx, "create signup ticket failed", "error", err)
		s.oauthFail(w, r, "sign-in failed, please try again")
		return
	}
	http.Redirect(w, r, "/oauth/complete?ticket="+url.QueryEscape(ticket)+
		"&provider="+url.PathEscape(external.ProviderKey)+
		"&name="+url.QueryEscape(external.Name), http.StatusFound)
}

func (s *AuthService) oauthRedirectURI(provider string) string {
	return s.publicURL + "/api/oauth/" + url.PathEscape(provider) + "/callback"
}

func (s *AuthService) stateCookie(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     oauthStateCookie,
		Value:    value,
		Path:     "/api/oauth/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	}
}

// oauthState is the per-flow secret packed into the httpOnly state cookie.
// State guards CSRF (it round-trips through the provider and is compared on
// callback); Nonce and Verifier back OIDC ID-token verification and PKCE and
// stay client-side only.
type oauthState struct {
	State    string `json:"s"`
	Nonce    string `json:"n,omitempty"`
	Verifier string `json:"v,omitempty"`
}

func encodeOauthState(st oauthState) (string, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeOauthState(value string) (oauthState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return oauthState{}, err
	}
	var st oauthState
	if err := json.Unmarshal(raw, &st); err != nil {
		return oauthState{}, err
	}
	return st, nil
}

// oauthFail lands on the login page with a coarse, non-technical reason.
func (s *AuthService) oauthFail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?oauthError="+url.QueryEscape(reason), http.StatusFound)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
