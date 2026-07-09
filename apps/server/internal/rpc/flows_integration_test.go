package rpc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"testing"
	"time"

	"connectrpc.com/connect"

	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/system/v1/systemv1connect"
	userv1 "github.com/imbytecat/moonbase/server/internal/gen/user/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/user/v1/userv1connect"
)

const mailpitAPI = "http://localhost:8025"

// requireMailpit skips unless the local mailpit dev container is reachable —
// keeps `go test` green on machines without docker compose up.
func requireMailpit(t *testing.T) {
	t.Helper()
	resp, err := http.Get(mailpitAPI + "/api/v1/info")
	if err != nil {
		t.Skip("mailpit not reachable; skipping email flow test")
	}
	_ = resp.Body.Close()
}

func configureSmtp(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	sys := systemv1connect.NewSystemServiceClient(client, baseURL)
	created, err := sys.CreateEmailProfile(t.Context(), connect.NewRequest(&systemv1.CreateEmailProfileRequest{
		Profile: &systemv1.Profile{
			Name:     "test smtp",
			Provider: "smtp",
			Config: mustStruct(t, map[string]any{
				"fromAddress": "noreply@example.com",
				"fromName":    "test",
				"host":        "localhost",
				"port":        float64(1025),
				"encryption":  "none",
			}),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sys.BindEmailPurpose(t.Context(), connect.NewRequest(&systemv1.BindEmailPurposeRequest{
		Purpose:   "auth",
		ProfileId: created.Msg.GetProfile().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
}

func latestMailTo(t *testing.T, to string) string {
	t.Helper()
	resp, err := http.Get(mailpitAPI + "/api/v1/search?query=" + url.QueryEscape("to:"+to))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var list struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Messages) == 0 {
		t.Fatalf("no mail delivered to %s", to)
	}
	msgResp, err := http.Get(fmt.Sprintf("%s/api/v1/message/%s", mailpitAPI, list.Messages[0].ID))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = msgResp.Body.Close() }()
	var msg struct {
		Text string `json:"Text"`
	}
	if err := json.NewDecoder(msgResp.Body).Decode(&msg); err != nil {
		t.Fatal(err)
	}
	return msg.Text
}

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// newMailUser provisions a throwaway email-bearing account through the real
// admin API (the seeded admin has no email by design) and returns a logged-in
// client for it. Unique per test run so reruns against the same database
// never collide.
func newMailUser(t *testing.T, baseURL string, admin *http.Client) (email, password string, client *http.Client) {
	t.Helper()
	email = fmt.Sprintf("mailflow-%d@example.com", time.Now().UnixNano())
	password = "mail-flow-pass-1"

	userAdmin := userv1connect.NewUserServiceClient(admin, baseURL)
	created, err := userAdmin.CreateUser(t.Context(), connect.NewRequest(&userv1.CreateUserRequest{
		Email:    email,
		Name:     "Mail Flow",
		Password: password,
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = userAdmin.DeleteUser(context.Background(),
			connect.NewRequest(&userv1.DeleteUserRequest{Id: created.Msg.GetUser().GetId()}))
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client = &http.Client{Jar: jar}
	authClient := authv1connect.NewAuthServiceClient(client, baseURL)
	if _, err := authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: email,
		Password:   password,
	})); err != nil {
		t.Fatal(err)
	}
	return email, password, client
}

func TestPasswordResetFlow(t *testing.T) {
	requireMailpit(t)
	baseURL, admin := newStack(t)
	loginAsAdmin(t, baseURL, admin)
	configureSmtp(t, baseURL, admin)
	email, password, userClient := newMailUser(t, baseURL, admin)

	authClient := authv1connect.NewAuthServiceClient(http.DefaultClient, baseURL)

	// Unknown email answers ok — no enumeration.
	if _, err := authClient.RequestPasswordReset(t.Context(),
		connect.NewRequest(&authv1.RequestPasswordResetRequest{Email: "ghost@example.com"})); err != nil {
		t.Fatalf("unknown email must still answer ok: %v", err)
	}

	if _, err := authClient.RequestPasswordReset(t.Context(),
		connect.NewRequest(&authv1.RequestPasswordResetRequest{Email: email})); err != nil {
		t.Fatal(err)
	}
	body := latestMailTo(t, email)
	m := tokenRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no token link in mail body: %q", body)
	}
	token := m[1]

	// Garbage token is rejected without detail.
	_, err := authClient.ResetPassword(t.Context(), connect.NewRequest(&authv1.ResetPasswordRequest{
		Token:       "not-a-real-token-aaaaaaaaaaaa",
		NewPassword: "brand-new-password-1",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad token: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := authClient.ResetPassword(t.Context(), connect.NewRequest(&authv1.ResetPasswordRequest{
		Token:       token,
		NewPassword: "brand-new-password-1",
	})); err != nil {
		t.Fatal(err)
	}

	// Token is single-use.
	_, err = authClient.ResetPassword(t.Context(), connect.NewRequest(&authv1.ResetPasswordRequest{
		Token:       token,
		NewPassword: "another-password-2",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("reused token: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	// Old password dead, new password works, and the user's previous session
	// was revoked by the reset.
	_, err = authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: email,
		Password:   password,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("old password: code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: email,
		Password:   "brand-new-password-1",
	})); err != nil {
		t.Fatal(err)
	}
	userAuth := authv1connect.NewAuthServiceClient(userClient, baseURL)
	if _, err := userAuth.GetMe(t.Context(), connect.NewRequest(&authv1.GetMeRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("pre-reset session must be revoked, got %v", err)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	requireMailpit(t)
	baseURL, admin := newStack(t)
	loginAsAdmin(t, baseURL, admin)
	configureSmtp(t, baseURL, admin)
	email, _, userClient := newMailUser(t, baseURL, admin)
	userAuth := authv1connect.NewAuthServiceClient(userClient, baseURL)

	me, err := userAuth.GetMe(t.Context(), connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if me.Msg.GetUser().GetEmailVerified() {
		t.Fatal("admin-created accounts must start unverified")
	}

	if _, err := userAuth.SendVerificationEmail(t.Context(),
		connect.NewRequest(&authv1.SendVerificationEmailRequest{})); err != nil {
		t.Fatal(err)
	}
	body := latestMailTo(t, email)
	m := tokenRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no token link in mail body: %q", body)
	}

	public := authv1connect.NewAuthServiceClient(http.DefaultClient, baseURL)
	if _, err := public.VerifyEmail(context.Background(),
		connect.NewRequest(&authv1.VerifyEmailRequest{Token: m[1]})); err != nil {
		t.Fatal(err)
	}

	me, err = userAuth.GetMe(t.Context(), connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if !me.Msg.GetUser().GetEmailVerified() {
		t.Fatal("email must be verified after consuming the link")
	}
}
