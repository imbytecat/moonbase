package rpc

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
	"github.com/imbytecat/moonbase/server/internal/verify"
)

type fakeAuthQuerier struct {
	repository.Querier
	getUserByEmail     func(ctx context.Context, email string) (repository.User, error)
	getUserByUsername  func(ctx context.Context, username string) (repository.User, error)
	getUserByPhone     func(ctx context.Context, phone string) (repository.User, error)
	createSession      func(ctx context.Context, arg repository.CreateSessionParams) (repository.Session, error)
	getSessionIdentity func(ctx context.Context, tokenHash []byte) (repository.GetSessionIdentityRow, error)
	getSetting         func(ctx context.Context, key string) (repository.Setting, error)
	getUser            func(ctx context.Context, id uuid.UUID) (repository.User, error)
	updatePassword     func(ctx context.Context, arg repository.UpdateUserPasswordParams) error
	deleteOtherSess    func(ctx context.Context, arg repository.DeleteOtherUserSessionsParams) error
	clearUserPhone     func(ctx context.Context, id uuid.UUID) (int64, error)
	deleteSessionByID  func(ctx context.Context, arg repository.DeleteUserSessionByIDParams) (int64, error)
	consumeTicket      func(ctx context.Context, secretHash []byte) (repository.OauthSignupTicket, error)
	deleteIdentity     func(ctx context.Context, arg repository.DeleteUserIdentityParams) (int64, error)
	updateUser         func(ctx context.Context, arg repository.UpdateUserParams) (repository.User, error)
	getFile            func(ctx context.Context, id uuid.UUID) (repository.File, error)
	setUserAvatar      func(ctx context.Context, arg repository.SetUserAvatarParams) error
}

func (f *fakeAuthQuerier) GetUserByEmail(
	ctx context.Context,
	email string,
) (repository.User, error) {
	return f.getUserByEmail(ctx, email)
}

func (f *fakeAuthQuerier) GetUserByUsername(
	ctx context.Context,
	username string,
) (repository.User, error) {
	return f.getUserByUsername(ctx, username)
}

func (f *fakeAuthQuerier) GetUserByPhone(
	ctx context.Context,
	phone string,
) (repository.User, error) {
	return f.getUserByPhone(ctx, phone)
}

func (f *fakeAuthQuerier) CreateSession(
	ctx context.Context,
	arg repository.CreateSessionParams,
) (repository.Session, error) {
	return f.createSession(ctx, arg)
}

func (f *fakeAuthQuerier) GetSessionIdentity(
	ctx context.Context,
	tokenHash []byte,
) (repository.GetSessionIdentityRow, error) {
	return f.getSessionIdentity(ctx, tokenHash)
}

func (f *fakeAuthQuerier) GetSetting(ctx context.Context, key string) (repository.Setting, error) {
	if f.getSetting == nil {
		return repository.Setting{}, pgx.ErrNoRows
	}
	return f.getSetting(ctx, key)
}

func (f *fakeAuthQuerier) GetUserMfa(context.Context, uuid.UUID) (repository.UserMfa, error) {
	return repository.UserMfa{}, pgx.ErrNoRows
}

func (f *fakeAuthQuerier) GetUser(ctx context.Context, id uuid.UUID) (repository.User, error) {
	return f.getUser(ctx, id)
}

func (f *fakeAuthQuerier) UpdateUserPassword(
	ctx context.Context,
	arg repository.UpdateUserPasswordParams,
) error {
	return f.updatePassword(ctx, arg)
}

func (f *fakeAuthQuerier) DeleteOtherUserSessions(
	ctx context.Context,
	arg repository.DeleteOtherUserSessionsParams,
) error {
	return f.deleteOtherSess(ctx, arg)
}

func (f *fakeAuthQuerier) ClearUserPhone(ctx context.Context, id uuid.UUID) (int64, error) {
	return f.clearUserPhone(ctx, id)
}

func (f *fakeAuthQuerier) DeleteUserSessionByID(
	ctx context.Context,
	arg repository.DeleteUserSessionByIDParams,
) (int64, error) {
	return f.deleteSessionByID(ctx, arg)
}

func (f *fakeAuthQuerier) UpdateUser(
	ctx context.Context,
	arg repository.UpdateUserParams,
) (repository.User, error) {
	return f.updateUser(ctx, arg)
}

func (f *fakeAuthQuerier) GetFile(ctx context.Context, id uuid.UUID) (repository.File, error) {
	return f.getFile(ctx, id)
}

func (f *fakeAuthQuerier) SetUserAvatar(
	ctx context.Context,
	arg repository.SetUserAvatarParams,
) error {
	return f.setUserAvatar(ctx, arg)
}

type allowAllCaptcha struct{}

func (allowAllCaptcha) Enabled(context.Context, string) (bool, error)        { return false, nil }
func (allowAllCaptcha) Verify(context.Context, string, string, string) error { return nil }
func (allowAllCaptcha) Widget(context.Context, string) (string, string, bool, error) {
	return "", "", false, nil
}

func newAuthService(q repository.Querier) *AuthService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAuthService(AuthServiceDeps{
		Repo:        q,
		Settings:    settings.NewStore(q),
		Captcha:     allowAllCaptcha{},
		SmsRegistry: sms.NewRegistry(),
		Verifier:    verify.NewService(q),
		Logger:      logger,
		Policy:      auth.SessionPolicy{TTL: time.Hour, MaxLifetime: 24 * time.Hour},
		PublicURL:   "http://localhost:5173",
	})
}

func TestLoginWrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	svc := newAuthService(&fakeAuthQuerier{
		getUserByEmail: func(context.Context, string) (repository.User, error) {
			return repository.User{ID: uuid.New(), PasswordHash: hash, IsActive: true}, nil
		},
	})

	_, err = svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "user@example.com",
		Password:   "wrong-password",
	}))

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestLoginUnknownEmailSameError(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{
		getUserByEmail: func(context.Context, string) (repository.User, error) {
			return repository.User{}, pgx.ErrNoRows
		},
	})

	_, err := svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "nobody@example.com",
		Password:   "whatever-password",
	}))

	// Same code and message as a wrong password — no account enumeration.
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestLoginRoutesIdentifierByShape(t *testing.T) {
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	user := repository.User{ID: uuid.New(), PasswordHash: hash, IsActive: true}
	var gotEmail, gotUsername, gotPhone string
	q := &fakeAuthQuerier{
		getUserByEmail: func(_ context.Context, email string) (repository.User, error) {
			gotEmail = email
			return user, nil
		},
		getUserByUsername: func(_ context.Context, username string) (repository.User, error) {
			gotUsername = username
			return user, nil
		},
		getUserByPhone: func(_ context.Context, phone string) (repository.User, error) {
			gotPhone = phone
			return user, nil
		},
		createSession: func(_ context.Context, arg repository.CreateSessionParams) (repository.Session, error) {
			return repository.Session{ID: uuid.New(), UserID: arg.UserID}, nil
		},
		getSessionIdentity: func(context.Context, []byte) (repository.GetSessionIdentityRow, error) {
			return repository.GetSessionIdentityRow{SessionID: uuid.New(), UserID: user.ID}, nil
		},
	}
	svc := newAuthService(q)

	login := func(identifier string) {
		t.Helper()
		if _, err := svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
			Identifier: identifier,
			Password:   "the-real-password",
		})); err != nil {
			t.Fatalf("login with %q: %v", identifier, err)
		}
	}

	login("user@example.com")
	if gotEmail != "user@example.com" {
		t.Fatalf("email lookup = %q, want user@example.com", gotEmail)
	}
	login("alice")
	if gotUsername != "alice" {
		t.Fatalf("username lookup = %q, want alice", gotUsername)
	}
	login("+8613800138000")
	if gotPhone != "+8613800138000" {
		t.Fatalf("phone lookup = %q, want +8613800138000 (E.164)", gotPhone)
	}
}

func TestLoginUnparseablePhoneSameError(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{})

	_, err := svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "+999999",
		Password:   "whatever-password",
	}))

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestLoginInactiveUserRejected(t *testing.T) {
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	svc := newAuthService(&fakeAuthQuerier{
		getUserByEmail: func(context.Context, string) (repository.User, error) {
			return repository.User{ID: uuid.New(), PasswordHash: hash, IsActive: false}, nil
		},
	})

	_, err = svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "disabled@example.com",
		Password:   "the-real-password",
	}))

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestLoginSetsSessionCookie(t *testing.T) {
	userID := uuid.New()
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	var storedHash []byte
	svc := newAuthService(&fakeAuthQuerier{
		getUserByEmail: func(context.Context, string) (repository.User, error) {
			return repository.User{
				ID:           userID,
				Email:        "user@example.com",
				PasswordHash: hash,
				IsActive:     true,
			}, nil
		},
		createSession: func(_ context.Context, arg repository.CreateSessionParams) (repository.Session, error) {
			storedHash = arg.TokenHash
			return repository.Session{
				ID:        uuid.New(),
				UserID:    arg.UserID,
				TokenHash: arg.TokenHash,
			}, nil
		},
		getSessionIdentity: func(context.Context, []byte) (repository.GetSessionIdentityRow, error) {
			return repository.GetSessionIdentityRow{
				SessionID:   uuid.New(),
				UserID:      userID,
				Email:       "user@example.com",
				Name:        "User",
				Permissions: []string{"report.read"},
			}, nil
		},
	})

	resp, err := svc.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "user@example.com",
		Password:   "the-real-password",
	}))
	if err != nil {
		t.Fatal(err)
	}

	cookie := resp.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("login must set a session cookie")
	}
	for _, want := range []string{"session=", "HttpOnly", "SameSite=Lax", "Path=/"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("cookie %q missing %q", cookie, want)
		}
	}
	if len(storedHash) == 0 {
		t.Fatal("session must be persisted with a token hash")
	}
	if got := resp.Msg.GetUser().
		GetPermissions(); len(got) != 1 ||
		got[0] != authv1.Permission_PERMISSION_REPORT_READ {
		t.Fatalf("permissions = %v, want [report.read]", got)
	}
}

func TestGetMeUsesInjectedIdentity(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{})
	id := &auth.Identity{
		UserID:      uuid.New(),
		Email:       "user@example.com",
		Name:        "User",
		Permissions: auth.PermissionSet("report.read", "user.read"),
	}

	resp, err := svc.GetMe(
		auth.WithIdentity(t.Context(), id),
		connect.NewRequest(&authv1.GetMeRequest{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Msg.GetUser().GetEmail() != "user@example.com" {
		t.Fatalf("email = %q", resp.Msg.GetUser().GetEmail())
	}
	if len(resp.Msg.GetUser().GetPermissions()) != 2 {
		t.Fatalf("permissions = %v, want 2 entries", resp.Msg.GetUser().GetPermissions())
	}
}

func TestRegisterDisabled(t *testing.T) {
	svc := newAuthService(&fakeAuthQuerier{
		getSetting: func(context.Context, string) (repository.Setting, error) {
			return repository.Setting{}, pgx.ErrNoRows
		},
	})

	_, err := svc.Register(t.Context(), connect.NewRequest(&authv1.RegisterRequest{
		Email:    "new@example.com",
		Name:     "New User",
		Password: "some-password",
	}))

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf(
			"code = %v, want permission_denied (registration defaults to disabled)",
			connect.CodeOf(err),
		)
	}
}

func TestSignupIdentifierValues(t *testing.T) {
	tests := []struct {
		name    string
		cfg     settings.Auth
		msg     *authv1.RegisterRequest
		wantErr bool
	}{
		{"username policy accepts username", settings.Auth{SignupIdentifiers: []string{"username"}},
			&authv1.RegisterRequest{Username: "alice"}, false},
		{
			"username policy rejects missing username",
			settings.Auth{SignupIdentifiers: []string{"username"}},
			&authv1.RegisterRequest{},
			true,
		},
		{"username policy rejects email", settings.Auth{SignupIdentifiers: []string{"username"}},
			&authv1.RegisterRequest{Username: "alice", Email: "a@example.com"}, true},
		{"email policy accepts email+code", settings.Auth{SignupIdentifiers: []string{"email"}},
			&authv1.RegisterRequest{Email: "a@example.com", EmailCode: "123456"}, false},
		{
			"email policy rejects username",
			settings.Auth{SignupIdentifiers: []string{"email"}},
			&authv1.RegisterRequest{
				Username:  "alice",
				Email:     "a@example.com",
				EmailCode: "123456",
			},
			true,
		},
		{
			"both policy requires both",
			settings.Auth{SignupIdentifiers: []string{"username", "email"}},
			&authv1.RegisterRequest{Username: "alice"},
			true,
		},
		{
			"both policy accepts both",
			settings.Auth{SignupIdentifiers: []string{"username", "email"}},
			&authv1.RegisterRequest{
				Username:  "alice",
				Email:     "a@example.com",
				EmailCode: "123456",
			},
			false,
		},
		{"phone policy requires phone", settings.Auth{SignupIdentifiers: []string{"phone"}},
			&authv1.RegisterRequest{}, true},
		{"phone policy requires code", settings.Auth{SignupIdentifiers: []string{"phone"}},
			&authv1.RegisterRequest{Phone: "+8613800138000"}, true},
		{"phone policy accepts phone+code", settings.Auth{SignupIdentifiers: []string{"phone"}},
			&authv1.RegisterRequest{Phone: "+8613800138000", PhoneCode: "123456"}, false},
		{"username policy rejects phone", settings.Auth{SignupIdentifiers: []string{"username"}},
			&authv1.RegisterRequest{Username: "alice", Phone: "+8613800138000"}, true},
		{"email policy requires code", settings.Auth{SignupIdentifiers: []string{"email"}},
			&authv1.RegisterRequest{Email: "a@example.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := signupIdentifierValues(tt.cfg, tt.msg)
			if tt.wantErr {
				if connect.CodeOf(err) != connect.CodeInvalidArgument {
					t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if values.username != tt.msg.GetUsername() || values.email != tt.msg.GetEmail() ||
				values.phone != tt.msg.GetPhone() {
				t.Fatalf("values = %+v, want fields from %+v", values, tt.msg)
			}
		})
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	userID := uuid.New()
	sessionID := uuid.New()
	hash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	var revoked *repository.DeleteOtherUserSessionsParams
	svc := newAuthService(&fakeAuthQuerier{
		getUser: func(context.Context, uuid.UUID) (repository.User, error) {
			return repository.User{ID: userID, PasswordHash: hash, IsActive: true}, nil
		},
		updatePassword: func(context.Context, repository.UpdateUserPasswordParams) error {
			return nil
		},
		deleteOtherSess: func(_ context.Context, arg repository.DeleteOtherUserSessionsParams) error {
			revoked = &arg
			return nil
		},
	})

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID, SessionID: sessionID})
	_, err = svc.ChangePassword(ctx, connect.NewRequest(&authv1.ChangePasswordRequest{
		CurrentPassword: "old-password",
		NewPassword:     "new-password-123",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if revoked == nil {
		t.Fatal("other sessions must be revoked")
	}
	if revoked.UserID != userID || revoked.ID != sessionID {
		t.Fatalf("revoked %+v, want user %s keeping session %s", revoked, userID, sessionID)
	}
}

func TestUnbindPhoneGuards(t *testing.T) {
	userID := uuid.New()
	hash, err := auth.HashPassword("the-real-password")
	if err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID, Phone: "+8613800138000"})

	t.Run("wrong password rejected", func(t *testing.T) {
		svc := newAuthService(&fakeAuthQuerier{
			getUser: func(context.Context, uuid.UUID) (repository.User, error) {
				return repository.User{ID: userID, PasswordHash: hash}, nil
			},
		})
		_, err := svc.UnbindPhone(ctx, connect.NewRequest(&authv1.UnbindPhoneRequest{
			CurrentPassword: "wrong-password-1",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
		}
	})

	t.Run("last identifier protected", func(t *testing.T) {
		svc := newAuthService(&fakeAuthQuerier{
			getUser: func(context.Context, uuid.UUID) (repository.User, error) {
				return repository.User{ID: userID, PasswordHash: hash}, nil
			},
			clearUserPhone: func(context.Context, uuid.UUID) (int64, error) {
				return 0, nil
			},
		})
		_, err := svc.UnbindPhone(ctx, connect.NewRequest(&authv1.UnbindPhoneRequest{
			CurrentPassword: "the-real-password",
		}))
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("code = %v, want failed_precondition", connect.CodeOf(err))
		}
	})

	t.Run("success clears phone", func(t *testing.T) {
		svc := newAuthService(&fakeAuthQuerier{
			getUser: func(context.Context, uuid.UUID) (repository.User, error) {
				return repository.User{ID: userID, PasswordHash: hash}, nil
			},
			clearUserPhone: func(context.Context, uuid.UUID) (int64, error) {
				return 1, nil
			},
		})
		resp, err := svc.UnbindPhone(ctx, connect.NewRequest(&authv1.UnbindPhoneRequest{
			CurrentPassword: "the-real-password",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if resp.Msg.GetUser().GetPhone() != "" {
			t.Fatalf("phone = %q, want empty", resp.Msg.GetUser().GetPhone())
		}
	})
}

func TestRevokeMySessionScopedToCaller(t *testing.T) {
	userID := uuid.New()
	var gotUserID uuid.UUID
	svc := newAuthService(&fakeAuthQuerier{
		deleteSessionByID: func(_ context.Context, arg repository.DeleteUserSessionByIDParams) (int64, error) {
			gotUserID = arg.UserID
			return 0, nil
		},
	})
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})

	_, err := svc.RevokeMySession(ctx, connect.NewRequest(&authv1.RevokeMySessionRequest{
		Id: uuid.NewString(),
	}))

	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf(
			"code = %v, want not_found (someone else's session must look nonexistent)",
			connect.CodeOf(err),
		)
	}
	if gotUserID != userID {
		t.Fatalf("query scoped to %s, want caller %s", gotUserID, userID)
	}
}

// Saving a new avatar transfers the caller's single attachment slot to the file
// they uploaded: UpdateProfile validates ownership then calls SetUserAvatar with
// that file id. The DB CTE (integration-tested) does the actual old→new move.
func TestUpdateProfileTransfersAvatarToOwnedFile(t *testing.T) {
	userID := uuid.New()
	fileID := uuid.New()
	var got *repository.SetUserAvatarParams
	svc := newAuthService(&fakeAuthQuerier{
		updateUser: func(_ context.Context, arg repository.UpdateUserParams) (repository.User, error) {
			return repository.User{ID: arg.ID}, nil
		},
		getFile: func(_ context.Context, id uuid.UUID) (repository.File, error) {
			return repository.File{
				ID:         id,
				ObjectKey:  "avatars/self/new.png",
				UploadedBy: userID,
				Purpose:    "avatars",
			}, nil
		},
		setUserAvatar: func(_ context.Context, arg repository.SetUserAvatarParams) error {
			got = &arg
			return nil
		},
	})

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})
	if _, err := svc.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{
		AvatarFileId: proto.String(fileID.String()),
	})); err != nil {
		t.Fatal(err)
	}

	if got == nil {
		t.Fatal("a saved avatar must transfer the attachment via SetUserAvatar")
	}
	if !got.FileID.Valid || uuid.UUID(got.FileID.Bytes) != fileID {
		t.Fatalf("SetUserAvatar file id = %v, want %s", got.FileID, fileID)
	}
	if got.UserID != userID {
		t.Fatalf("SetUserAvatar user id = %s, want caller %s", got.UserID, userID)
	}
}

// A caller may only attach a file they uploaded as an avatar. A file owned by
// someone else is rejected and never attached — no borrowing another user's file.
func TestUpdateProfileRejectsUnownedAvatarFile(t *testing.T) {
	userID := uuid.New()
	otherID := uuid.New()
	attached := false
	svc := newAuthService(&fakeAuthQuerier{
		updateUser: func(_ context.Context, arg repository.UpdateUserParams) (repository.User, error) {
			return repository.User{ID: arg.ID}, nil
		},
		getFile: func(_ context.Context, id uuid.UUID) (repository.File, error) {
			return repository.File{
				ID:         id,
				ObjectKey:  "avatars/other/x.png",
				UploadedBy: otherID,
				Purpose:    "avatars",
			}, nil
		},
		setUserAvatar: func(context.Context, repository.SetUserAvatarParams) error {
			attached = true
			return nil
		},
	})

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})
	_, err := svc.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{
		AvatarFileId: proto.String(uuid.NewString()),
	}))

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf(
			"code = %v, want invalid_argument for a file the caller didn't upload",
			connect.CodeOf(err),
		)
	}
	if attached {
		t.Fatal("must not attach a file the caller didn't upload")
	}
}

// Clearing the avatar (empty file id) skips the file lookup and calls
// SetUserAvatar with a NULL file id, so the CTE drops the attachment.
func TestUpdateProfileClearsAvatar(t *testing.T) {
	userID := uuid.New()
	lookedUp := false
	var got *repository.SetUserAvatarParams
	svc := newAuthService(&fakeAuthQuerier{
		updateUser: func(_ context.Context, arg repository.UpdateUserParams) (repository.User, error) {
			return repository.User{ID: arg.ID}, nil
		},
		getFile: func(_ context.Context, _ uuid.UUID) (repository.File, error) {
			lookedUp = true
			return repository.File{}, nil
		},
		setUserAvatar: func(_ context.Context, arg repository.SetUserAvatarParams) error {
			got = &arg
			return nil
		},
	})

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})
	if _, err := svc.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{
		AvatarFileId: proto.String(""),
	})); err != nil {
		t.Fatal(err)
	}

	if lookedUp {
		t.Fatal("clearing the avatar must not look up a file")
	}
	if got == nil || got.FileID.Valid {
		t.Fatalf("clearing must call SetUserAvatar with a NULL file id, got %+v", got)
	}
}
