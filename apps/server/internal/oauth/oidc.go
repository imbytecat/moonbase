package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// Standard OpenID Connect authorization-code flow via coreos/go-oidc +
// golang.org/x/oauth2. Discovery (RFC 8414 / OIDC Discovery) resolves the
// endpoints and JWKS; the ID token is signature-verified against that JWKS and
// its issuer/audience/expiry checked; the flow is hardened with a nonce (bound
// into the ID token) and PKCE S256. The verified subject claim is the stable
// identity; ID-token and UserInfo claims supply display fields.
const oidcDefaultScopes = "openid profile email"

// providerCache avoids re-running discovery (endpoints + JWKS location) on
// every login; entries expire so issuer reconfiguration is picked up within
// minutes. A cached *oidc.Provider is safe to reuse across requests — Verify
// uses the per-call context for any JWKS refetch.
var providerCache = struct {
	sync.Mutex
	entries map[string]providerEntry
}{entries: map[string]providerEntry{}}

type providerEntry struct {
	provider *oidc.Provider
	fetched  time.Time
}

const providerTTL = 10 * time.Minute

func oidcProvider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	issuer = strings.TrimSuffix(issuer, "/")
	providerCache.Lock()
	entry, ok := providerCache.entries[issuer]
	providerCache.Unlock()
	if ok && time.Since(entry.fetched) < providerTTL {
		return entry.provider, nil
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	providerCache.Lock()
	providerCache.entries[issuer] = providerEntry{provider: provider, fetched: time.Now()}
	providerCache.Unlock()
	return provider, nil
}

// oidcScopes forces the openid scope: without it the provider returns no ID
// token and there is nothing to verify.
func oidcScopes(p systemcodec.OauthProfile) []string {
	raw := p.Oidc.Scopes
	if raw == "" {
		raw = oidcDefaultScopes
	}
	scopes := strings.Fields(raw)
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	return scopes
}

func oidcOauth2Config(p systemcodec.OauthProfile, provider *oidc.Provider, redirectURI string) oauth2.Config {
	return oauth2.Config{
		ClientID:     p.Oidc.ClientId,
		ClientSecret: p.Oidc.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       oidcScopes(p),
	}
}

func oidcAuthorizeURL(ctx context.Context, p systemcodec.OauthProfile, redirectURI, state string) (string, FlowSecrets, error) {
	provider, err := oidcProvider(ctx, p.Oidc.Issuer)
	if err != nil {
		return "", FlowSecrets{}, err
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", FlowSecrets{}, err
	}
	verifier := oauth2.GenerateVerifier()
	cfg := oidcOauth2Config(p, provider, redirectURI)
	url := cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return url, FlowSecrets{Nonce: nonce, Verifier: verifier}, nil
}

func oidcExchange(ctx context.Context, p systemcodec.OauthProfile, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error) {
	provider, err := oidcProvider(ctx, p.Oidc.Issuer)
	if err != nil {
		return ExternalIdentity{}, err
	}
	cfg := oidcOauth2Config(p, provider, redirectURI)

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(secrets.Verifier))
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("oidc token exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return ExternalIdentity{}, fmt.Errorf("oidc token exchange: response carried no id_token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: p.Oidc.ClientId}).Verify(ctx, rawIDToken)
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("oidc id_token verify: %w", err)
	}
	if idToken.Nonce != secrets.Nonce {
		return ExternalIdentity{}, fmt.Errorf("oidc id_token: nonce mismatch")
	}

	var claims struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return ExternalIdentity{}, fmt.Errorf("oidc id_token claims: %w", err)
	}
	// The ID token authenticates the subject; profile fields it omits are
	// pulled from UserInfo (best-effort — a missing name never fails login).
	if claims.Name == "" || claims.Picture == "" {
		if info, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
			_ = info.Claims(&claims)
		}
	}

	name := claims.Name
	if name == "" {
		name = claims.Email
	}
	return ExternalIdentity{
		ProviderID: idToken.Subject,
		Name:       name,
		AvatarURL:  claims.Picture,
	}, nil
}

func randomNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
