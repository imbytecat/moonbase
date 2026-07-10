package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
	"golang.org/x/oauth2"
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
	provider *coreoidc.Provider
	fetched  time.Time
}

const providerTTL = 10 * time.Minute

func oidcProvider(ctx context.Context, issuer string) (*coreoidc.Provider, error) {
	issuer = strings.TrimSuffix(issuer, "/")
	providerCache.Lock()
	entry, ok := providerCache.entries[issuer]
	providerCache.Unlock()
	if ok && time.Since(entry.fetched) < providerTTL {
		return entry.provider, nil
	}
	provider, err := coreoidc.NewProvider(ctx, issuer)
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
func oidcScopes(config providerConfig) []string {
	raw := config.Scopes
	if raw == "" {
		raw = oidcDefaultScopes
	}
	scopes := strings.Fields(raw)
	if !slices.Contains(scopes, coreoidc.ScopeOpenID) {
		scopes = append([]string{coreoidc.ScopeOpenID}, scopes...)
	}
	return scopes
}

func oidcOauth2Config(
	config providerConfig,
	provider *coreoidc.Provider,
	redirectURI string,
) oauth2.Config {
	return oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       oidcScopes(config),
	}
}

func oidcAuthorizeURL(
	ctx context.Context,
	config providerConfig,
	redirectURI, state string,
) (string, oauthint.FlowSecrets, error) {
	provider, err := oidcProvider(ctx, config.Issuer)
	if err != nil {
		return "", oauthint.FlowSecrets{}, err
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", oauthint.FlowSecrets{}, err
	}
	verifier := oauth2.GenerateVerifier()
	cfg := oidcOauth2Config(config, provider, redirectURI)
	url := cfg.AuthCodeURL(state, coreoidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return url, oauthint.FlowSecrets{Nonce: nonce, Verifier: verifier}, nil
}

func oidcExchange(
	ctx context.Context,
	config providerConfig,
	code, redirectURI string,
	secrets oauthint.FlowSecrets,
) (oauthint.ExternalIdentity, error) {
	provider, err := oidcProvider(ctx, config.Issuer)
	if err != nil {
		return oauthint.ExternalIdentity{}, err
	}
	cfg := oidcOauth2Config(config, provider, redirectURI)

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(secrets.Verifier))
	if err != nil {
		return oauthint.ExternalIdentity{}, fmt.Errorf("oidc token exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return oauthint.ExternalIdentity{}, fmt.Errorf(
			"oidc token exchange: response carried no id_token",
		)
	}
	idToken, err := provider.Verifier(&coreoidc.Config{ClientID: config.ClientID}).
		Verify(ctx, rawIDToken)
	if err != nil {
		return oauthint.ExternalIdentity{}, fmt.Errorf("oidc id_token verify: %w", err)
	}
	if idToken.Nonce != secrets.Nonce {
		return oauthint.ExternalIdentity{}, fmt.Errorf("oidc id_token: nonce mismatch")
	}

	var claims struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return oauthint.ExternalIdentity{}, fmt.Errorf("oidc id_token claims: %w", err)
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
	return oauthint.ExternalIdentity{
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
