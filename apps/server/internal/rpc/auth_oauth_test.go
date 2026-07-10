package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

type fakeOauthFlow struct {
	authorizeURL string
	external     oauth.ExternalIdentity
	err          error
}

func (f *fakeOauthFlow) AuthorizeURL(context.Context, string, string, string) (string, oauth.FlowSecrets, error) {
	if f.err != nil {
		return "", oauth.FlowSecrets{}, f.err
	}
	return f.authorizeURL, oauth.FlowSecrets{}, nil
}

func (f *fakeOauthFlow) Exchange(context.Context, string, string, string, oauth.FlowSecrets) (oauth.ExternalIdentity, error) {
	return f.external, f.err
}

func (f *fakeOauthFlow) ProviderOptions(context.Context) ([]oauth.ProviderOption, error) {
	return nil, f.err
}

func (f *fakeAuthQuerier) ConsumeOauthSignupTicket(ctx context.Context, secretHash []byte) (repository.OauthSignupTicket, error) {
	return f.consumeTicket(ctx, secretHash)
}

func (f *fakeAuthQuerier) DeleteUserIdentity(ctx context.Context, arg repository.DeleteUserIdentityParams) (int64, error) {
	return f.deleteIdentity(ctx, arg)
}

func TestCompleteOauthSignupInvalidTicket(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{
		getSetting: settingJSON(t, map[string]string{
			"auth": `{"registrationEnabled":true,"signupIdentifiers":["username"]}`,
		}),
		consumeTicket: func(context.Context, []byte) (repository.OauthSignupTicket, error) {
			return repository.OauthSignupTicket{}, pgx.ErrNoRows
		},
	})

	_, err := svc.CompleteOauthSignup(t.Context(), connect.NewRequest(&authv1.CompleteOauthSignupRequest{
		Ticket:   "definitely-not-a-real-ticket",
		Name:     "Neo",
		Username: "neo",
		Password: "password-123",
	}))

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestCompleteOauthSignupRegistrationDisabled(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{
		getSetting: func(context.Context, string) (repository.Setting, error) {
			return repository.Setting{}, pgx.ErrNoRows
		},
	})

	_, err := svc.CompleteOauthSignup(t.Context(), connect.NewRequest(&authv1.CompleteOauthSignupRequest{
		Ticket:   "whatever-ticket-value",
		Name:     "Neo",
		Username: "neo",
		Password: "password-123",
	}))

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("code = %v, want permission_denied", connect.CodeOf(err))
	}
}

func TestUnbindOauthIdentityGuards(t *testing.T) {
	userID := uuid.New()
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})

	t.Run("wrong password rejected", func(t *testing.T) {
		svc := newAuthService(&fakeAuthQuerier{
			getUser: func(context.Context, uuid.UUID) (repository.User, error) {
				return repository.User{ID: userID, PasswordHash: hash}, nil
			},
		})
		_, err := svc.UnbindOauthIdentity(ctx, connect.NewRequest(&authv1.UnbindOauthIdentityRequest{
			ProviderKey:     "wechat",
			CurrentPassword: "wrong-password-1",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
		}
	})

	t.Run("scoped to caller", func(t *testing.T) {
		var gotUserID uuid.UUID
		svc := newAuthService(&fakeAuthQuerier{
			getUser: func(context.Context, uuid.UUID) (repository.User, error) {
				return repository.User{ID: userID, PasswordHash: hash}, nil
			},
			deleteIdentity: func(_ context.Context, arg repository.DeleteUserIdentityParams) (int64, error) {
				gotUserID = arg.UserID
				return 0, nil
			},
		})
		_, err := svc.UnbindOauthIdentity(ctx, connect.NewRequest(&authv1.UnbindOauthIdentityRequest{
			ProviderKey:     "wechat",
			CurrentPassword: "the-real-password",
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("code = %v, want not_found", connect.CodeOf(err))
		}
		if gotUserID != userID {
			t.Fatalf("query scoped to %s, want caller %s", gotUserID, userID)
		}
	})
}

func TestOauthCallbackStateMismatch(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{})

	legit, err := encodeOauthState(oauthState{State: "legit"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/wechat/callback?state=forged&code=x", nil)
	req.SetPathValue("provider", "wechat")
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: legit})
	rec := httptest.NewRecorder()

	svc.OauthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?oauthError=") {
		t.Fatalf("redirect = %q, want /login?oauthError=...", loc)
	}
}

func TestOauthAuthorizeSetsStateAndRedirects(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{})
	svc.oauth = &fakeOauthFlow{authorizeURL: "https://open.weixin.qq.com/connect/qrconnect?appid=x"}

	req := httptest.NewRequest(http.MethodGet, "/oauth/wechat/authorize", nil)
	req.SetPathValue("provider", "wechat")
	rec := httptest.NewRecorder()

	svc.OauthAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "https://open.weixin.qq.com/") {
		t.Fatalf("redirect = %q, want the provider authorize page", got)
	}
	cookies := rec.Result().Cookies()
	var state string
	for _, c := range cookies {
		if c.Name == oauthStateCookie {
			state = c.Value
		}
	}
	if state == "" {
		t.Fatal("authorize must set the CSRF state cookie")
	}
}

func settingJSON(t *testing.T, values map[string]string) func(context.Context, string) (repository.Setting, error) {
	t.Helper()
	return func(_ context.Context, key string) (repository.Setting, error) {
		raw, ok := values[key]
		if !ok {
			return repository.Setting{}, pgx.ErrNoRows
		}
		return repository.Setting{Key: key, Value: []byte(raw)}, nil
	}
}
