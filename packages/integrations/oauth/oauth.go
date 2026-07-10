// Package oauth defines the typed provider execution model for an already
// selected third-party login profile. Application purposes, settings loading,
// binding checks and key routing belong to the consuming application facade.
package oauth

import (
	"context"
	"errors"
)

var ErrNotConfigured = errors.New("oauth provider is not configured")

type ExternalIdentity struct {
	ProviderKey string
	ProviderID  string
	Name        string
	AvatarURL   string
}

// FlowSecrets are minted at authorize time and returned by the caller at
// exchange time. OIDC uses nonce + PKCE verifier; WeChat leaves both empty.
type FlowSecrets struct {
	Nonce    string
	Verifier string
}

// Executor is the reusable selected-profile seam. ProviderKey assignment is
// application routing metadata and is deliberately not performed here.
type Executor interface {
	AuthorizeURL(context.Context, string, map[string]any, string, string) (string, FlowSecrets, error)
	Exchange(context.Context, string, map[string]any, string, string, FlowSecrets) (ExternalIdentity, error)
}

var _ Executor = Registry{}
