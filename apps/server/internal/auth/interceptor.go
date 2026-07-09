package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

const (
	cookieName = "session"
	// __Host- (browser-enforced) requires Secure, Path=/, no Domain — it pins
	// the cookie to this exact origin so a subdomain can never plant one.
	secureCookieName = "__Host-session"
)

// CookieName returns the session cookie name for the deployment mode. The
// middleware accepts both names so flipping secure_cookie doesn't strand
// existing sessions mid-rollout.
func CookieName(secure bool) string {
	if secure {
		return secureCookieName
	}
	return cookieName
}

// SessionPolicy is the sliding-expiry contract: sessions idle out after TTL,
// renewals extend by TTL again (only once less than half is left, to avoid a
// write per request), and MaxLifetime hard-caps total age from creation so a
// stolen token can't keep itself alive forever.
type SessionPolicy struct {
	TTL         time.Duration
	MaxLifetime time.Duration
}

// InitialExpiry is the idle expiry for a brand-new session (TTL, capped by
// MaxLifetime for pathological configs where TTL > MaxLifetime).
func (p SessionPolicy) InitialExpiry(now time.Time) time.Time {
	return p.capExpiry(now.Add(p.TTL), now)
}

func (p SessionPolicy) capExpiry(want time.Time, createdAt time.Time) time.Time {
	if p.MaxLifetime <= 0 {
		return want
	}
	if cap := createdAt.Add(p.MaxLifetime); want.After(cap) {
		return cap
	}
	return want
}

// renewedExpiry returns the new expiry and true when the session has consumed
// more than half its idle window; otherwise false (no DB write needed).
func (p SessionPolicy) renewedExpiry(now, expiresAt, createdAt time.Time) (time.Time, bool) {
	if expiresAt.Sub(now) > p.TTL/2 {
		return time.Time{}, false
	}
	next := p.capExpiry(now.Add(p.TTL), createdAt)
	if !next.After(expiresAt) {
		return time.Time{}, false
	}
	return next, true
}

// NewMiddleware builds the connectrpc.com/authn middleware that resolves the
// session token to an *Identity and slides its expiry per the policy.
// Browsers send the token via the httpOnly cookie; native apps send the same
// token as "Authorization: Bearer <token>" — one sessions table, one
// revocation story. It never rejects: unauthenticated requests proceed with
// no identity and the authz interceptor decides per-RPC.
func NewMiddleware(repo repository.Querier, logger *slog.Logger, policy SessionPolicy) *authn.Middleware {
	return authn.NewMiddleware(func(ctx context.Context, r *http.Request) (any, error) {
		token := sessionToken(r)
		if token == "" {
			return nil, nil //nolint:nilnil // no session: anonymous, not an error
		}
		row, err := repo.GetSessionIdentity(ctx, HashSessionToken(token))
		if err != nil {
			// Expired/unknown sessions are anonymous; real DB errors are logged
			// but still degrade to anonymous so public endpoints stay up.
			if !errors.Is(err, pgx.ErrNoRows) {
				logger.ErrorContext(ctx, "session lookup failed", "error", err)
			}
			return nil, nil //nolint:nilnil // degrade to anonymous
		}

		// Sliding renewal is best-effort: a failed write means the session
		// just expires on its old schedule, never a failed request.
		if next, ok := policy.renewedExpiry(time.Now(), row.ExpiresAt, row.SessionCreatedAt); ok {
			if err := repo.TouchSession(ctx, repository.TouchSessionParams{
				ID:        row.SessionID,
				ExpiresAt: next,
			}); err != nil {
				logger.ErrorContext(ctx, "session renewal failed", "error", err)
			}
		}

		return &Identity{
			UserID:        row.UserID,
			SessionID:     row.SessionID,
			Username:      row.Username,
			Email:         row.Email,
			Name:          row.Name,
			AvatarFileID:  row.AvatarFileID,
			Phone:         row.Phone,
			EmailVerified: row.EmailVerified,
			Permissions:   PermissionSet(row.Permissions...),
		}, nil
	})
}

func sessionToken(r *http.Request) string {
	if bearer, ok := authn.BearerToken(r); ok {
		return bearer
	}
	for _, name := range [2]string{secureCookieName, cookieName} {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

var (
	errUnauthenticated  = connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	errPermissionDenied = connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
)

// Rule is the access requirement for one RPC procedure.
type Rule struct {
	// Public procedures skip all checks (login, register, auth config).
	Public bool
	// Permission required, or "" for "any authenticated user" (e.g. GetMe).
	Permission string
}

// NewAuthzInterceptor enforces the procedure→Rule table. Unknown procedures
// are denied by default; the completeness test asserts the table covers every
// registered RPC, so adding one without an access decision fails CI instead
// of shipping unprotected.
func NewAuthzInterceptor(rules map[string]Rule) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if err := Authorize(rules, req.Spec().Procedure, IdentityFromContext(ctx)); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}
}

// Authorize is the pure access decision: nil to proceed, or a typed connect
// error. Split from the interceptor so it's testable without HTTP plumbing.
func Authorize(rules map[string]Rule, procedure string, id *Identity) error {
	rule, ok := rules[procedure]
	if !ok {
		return errPermissionDenied
	}
	if rule.Public {
		return nil
	}
	if id == nil {
		return errUnauthenticated
	}
	if rule.Permission != "" && !id.Can(rule.Permission) {
		return errPermissionDenied
	}
	return nil
}
