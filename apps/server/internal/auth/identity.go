// Package auth provides session-based authentication and RBAC authorization
// as two thin edge layers: connectrpc.com/authn middleware (cookie → Identity
// in context) and a Connect interceptor enforcing a declarative rule table.
// Business handlers never see cookies or tokens — they read a typed Identity
// from the context, which tests inject directly with WithIdentity. The rule
// table plus its completeness test guarantee no RPC ships without an access
// decision.
package auth

import (
	"context"

	"connectrpc.com/authn"
	"github.com/google/uuid"
)

// WildcardPermission grants every permission (assigned to the admin role).
const WildcardPermission = "*"

// Identity is the authenticated caller as seen by business code. Permissions
// is the union of the user's role permissions, resolved once per request.
type Identity struct {
	UserID        uuid.UUID
	SessionID     uuid.UUID
	Username      string
	Email         string
	Name          string
	AvatarFileID  string
	Phone         string
	EmailVerified bool
	Permissions   map[string]struct{}
}

// Can reports whether the identity holds perm (or the wildcard).
func (id *Identity) Can(perm string) bool {
	if _, ok := id.Permissions[WildcardPermission]; ok {
		return true
	}
	_, ok := id.Permissions[perm]
	return ok
}

// PermissionSet builds an Identity.Permissions value from permission keys.
// Tests use PermissionSet(auth.WildcardPermission) for an all-access fake.
func PermissionSet(perms ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return set
}

// WithIdentity returns a context carrying the identity, using authn's info
// slot — the same one the middleware fills for real requests. Tests call this
// to fake any caller.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return authn.SetInfo(ctx, id)
}

// IdentityFromContext returns the caller's identity, or nil when the request
// is unauthenticated.
func IdentityFromContext(ctx context.Context) *Identity {
	id, _ := authn.GetInfo(ctx).(*Identity)
	return id
}
