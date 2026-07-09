package oauth

import (
	"slices"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestOidcScopesForcesOpenID(t *testing.T) {
	tests := map[string]struct {
		configured string
		want       []string
	}{
		"empty defaults to openid profile email": {
			configured: "",
			want:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		"custom scopes gain openid when missing": {
			configured: "profile groups",
			want:       []string{oidc.ScopeOpenID, "profile", "groups"},
		},
		"openid not duplicated when present": {
			configured: "openid email",
			want:       []string{oidc.ScopeOpenID, "email"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := oidcScopes(map[string]any{"scopes": tc.configured})
			if !slices.Equal(got, tc.want) {
				t.Fatalf("oidcScopes(%q) = %v, want %v", tc.configured, got, tc.want)
			}
		})
	}
}
