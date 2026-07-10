package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/config"
	"github.com/imbytecat/moonbase/server/internal/database"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	reportv1 "github.com/imbytecat/moonbase/server/internal/gen/report/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/report/v1/reportv1connect"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	userv1 "github.com/imbytecat/moonbase/server/internal/gen/user/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/user/v1/userv1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/server"

	"net/http/httptest"
)

const (
	testAdminUsername = "admin"
	// 8+ chars: ChangePassword/ResetPassword enforce min_len 8.
	testAdminPassword = "admin123"
)

// newStack builds the REAL production handler chain — router, authn
// middleware, authz interceptor, protovalidate, embedded migrations, seed —
// against MOONBASE_DATABASE_URL. Without that env the test skips, so
// `go test ./...` stays green on machines with no Postgres.
func newStack(t *testing.T) (baseURL string, client *http.Client) {
	t.Helper()
	baseURL, client, _ = newStackWithPool(t)
	return baseURL, client
}

// newStackWithPool is newStack plus the backing pool, for integration tests
// that assert on persisted rows the RPC surface doesn't yet read back.
func newStackWithPool(t *testing.T) (baseURL string, client *http.Client, pool *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("MOONBASE_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONBASE_DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := database.NewPool(ctx, dsn, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}

	if err := auth.Seed(ctx, repository.New(pool), logger, testAdminUsername, testAdminPassword); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Auth.SessionTTLHours = 1
	cfg.Auth.SessionMaxLifetimeHours = 24
	handler := server.NewRouter(cfg, pool, nil, logger)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := srv.Client()
	httpClient.Jar = jar
	return srv.URL + "/api", httpClient, pool
}

// loginAsAdmin authenticates against the real Login RPC; the cookie jar then
// carries the session for every subsequent request on the same client. Uses
// the username identifier: it exists regardless of whether the database was
// seeded with an admin email (production default is email-less).
func loginAsAdmin(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	authClient := authv1connect.NewAuthServiceClient(client, baseURL)
	_, err := authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: testAdminUsername,
		Password:   testAdminPassword,
	}))
	if err != nil {
		t.Fatalf("admin login failed: %v", err)
	}
}

func mustConfigWrite(t *testing.T, config map[string]any) *systemv1.ConfigWrite {
	t.Helper()
	out, err := structpb.NewStruct(config)
	if err != nil {
		t.Fatal(err)
	}
	return &systemv1.ConfigWrite{Values: out}
}

func TestAuthFlowAndPermissions(t *testing.T) {
	baseURL, client := newStack(t)
	reportClient := reportv1connect.NewReportServiceClient(client, baseURL)
	authClient := authv1connect.NewAuthServiceClient(client, baseURL)

	// Unauthenticated: protected RPC → unauthenticated, public RPC → ok.
	_, err := reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetDashboardReport: code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := authClient.GetAuthConfig(t.Context(),
		connect.NewRequest(&authv1.GetAuthConfigRequest{})); err != nil {
		t.Fatalf("public GetAuthConfig must work logged out: %v", err)
	}

	// Wrong password → unauthenticated.
	_, err = authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: testAdminUsername,
		Password:   "definitely-wrong",
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("wrong password: code = %v, want unauthenticated", connect.CodeOf(err))
	}

	// Login → session cookie → GetMe returns the admin with wildcard perms.
	loginAsAdmin(t, baseURL, client)
	me, err := authClient.GetMe(t.Context(), connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if me.Msg.GetUser().GetUsername() != testAdminUsername {
		t.Fatalf("GetMe username = %q, want admin (seeded)", me.Msg.GetUser().GetUsername())
	}

	// Authenticated admin can hit protected RPCs.
	if _, err := reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30})); err != nil {
		t.Fatalf("admin GetDashboardReport: %v", err)
	}

	// Logout clears the session server-side; the next call fails again.
	if _, err := authClient.Logout(t.Context(),
		connect.NewRequest(&authv1.LogoutRequest{})); err != nil {
		t.Fatal(err)
	}
	_, err = reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("after logout: code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestRBACPermissionBoundary(t *testing.T) {
	baseURL, adminClient := newStack(t)
	loginAsAdmin(t, baseURL, adminClient)
	userAdmin := userv1connect.NewUserServiceClient(adminClient, baseURL)

	// Admin creates a user with NO roles → that user has no permissions.
	created, err := userAdmin.CreateUser(t.Context(), connect.NewRequest(&userv1.CreateUserRequest{
		Email:    "powerless@example.com",
		Name:     "Powerless",
		Password: "user-password-123",
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
	plainClient := &http.Client{Jar: jar}
	plainAuth := authv1connect.NewAuthServiceClient(plainClient, baseURL)
	if _, err := plainAuth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: "powerless@example.com",
		Password:   "user-password-123",
	})); err != nil {
		t.Fatal(err)
	}

	// Authenticated but permissionless: session-only RPC works…
	if _, err := plainAuth.GetMe(t.Context(),
		connect.NewRequest(&authv1.GetMeRequest{})); err != nil {
		t.Fatalf("GetMe for permissionless user: %v", err)
	}
	// …permissioned RPCs are denied with permission_denied (not unauthenticated).
	plainReports := reportv1connect.NewReportServiceClient(plainClient, baseURL)
	_, err = plainReports.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("report.read without role: code = %v, want permission_denied", connect.CodeOf(err))
	}
	plainUsers := userv1connect.NewUserServiceClient(plainClient, baseURL)
	_, err = plainUsers.ListUsers(t.Context(), connect.NewRequest(&userv1.ListUsersRequest{}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("user.read without role: code = %v, want permission_denied", connect.CodeOf(err))
	}
}

func TestBearerTokenFlow(t *testing.T) {
	baseURL, client := newStack(t)
	authClient := authv1connect.NewAuthServiceClient(client, baseURL)

	login, err := authClient.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Identifier: testAdminUsername,
		Password:   testAdminPassword,
	}))
	if err != nil {
		t.Fatal(err)
	}
	token := login.Msg.GetSessionToken()
	if token == "" {
		t.Fatal("login must return a session token for native clients")
	}

	// A cookie-less client using only Authorization: Bearer — the native-app
	// path. Same session, same revocation.
	bearer := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	})
	plainHTTP := &http.Client{}
	bearerAuth := authv1connect.NewAuthServiceClient(plainHTTP, baseURL, connect.WithInterceptors(bearer))
	bearerReports := reportv1connect.NewReportServiceClient(plainHTTP, baseURL, connect.WithInterceptors(bearer))

	me, err := bearerAuth.GetMe(t.Context(), connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatalf("GetMe via bearer: %v", err)
	}
	if me.Msg.GetUser().GetUsername() != testAdminUsername {
		t.Fatalf("bearer GetMe username = %q", me.Msg.GetUser().GetUsername())
	}
	if _, err := bearerReports.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30})); err != nil {
		t.Fatalf("GetDashboardReport via bearer: %v", err)
	}

	// Logout over Bearer revokes the session server-side.
	if _, err := bearerAuth.Logout(t.Context(),
		connect.NewRequest(&authv1.LogoutRequest{})); err != nil {
		t.Fatal(err)
	}
	_, err = bearerReports.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("after bearer logout: code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestDashboardReportAggregates(t *testing.T) {
	baseURL, client := newStack(t)
	loginAsAdmin(t, baseURL, client)
	reportClient := reportv1connect.NewReportServiceClient(client, baseURL)

	resp, err := reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 30}))
	if err != nil {
		t.Fatal(err)
	}

	// The seeded admin exists and just logged in, so the headline counts and
	// both time series must reflect at least that.
	if resp.Msg.GetTotalUsers() < 1 {
		t.Fatalf("total users = %d, want >= 1 (seeded admin)", resp.Msg.GetTotalUsers())
	}
	if resp.Msg.GetActiveSessions() < 1 {
		t.Fatalf("active sessions = %d, want >= 1 (this login)", resp.Msg.GetActiveSessions())
	}
	if len(resp.Msg.GetLogins()) == 0 {
		t.Fatal("logins series is empty despite a fresh login")
	}
	if len(resp.Msg.GetUsersByRole()) == 0 {
		t.Fatal("users-by-role breakdown is empty despite seeded roles")
	}
	for _, p := range resp.Msg.GetUserSignups() {
		if len(p.GetDate()) != len("2006-01-02") {
			t.Fatalf("signup point date = %q, want YYYY-MM-DD", p.GetDate())
		}
	}
}

func TestValidationRejectsAtTheEdge(t *testing.T) {
	baseURL, client := newStack(t)
	loginAsAdmin(t, baseURL, client)
	reportClient := reportv1connect.NewReportServiceClient(client, baseURL)

	_, err := reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 0}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("days=0: code = %v, want invalid_argument", connect.CodeOf(err))
	}

	_, err = reportClient.GetDashboardReport(t.Context(),
		connect.NewRequest(&reportv1.GetDashboardReportRequest{Days: 9999}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("days=9999: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}
