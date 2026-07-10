package auth

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("wrong password accepted")
	}
}

func TestSessionTokenHashIsStable(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if string(HashSessionToken(token)) != string(hash) {
		t.Fatal("HashSessionToken(token) must equal the hash returned at creation")
	}
}

func TestIdentityCan(t *testing.T) {
	cases := []struct {
		name  string
		perms []string
		check string
		want  bool
	}{
		{"granted", []string{"report.read"}, "report.read", true},
		{"missing", []string{"report.read"}, "user.write", false},
		{"wildcard", []string{WildcardPermission}, "anything.at.all", true},
		{"empty", nil, "report.read", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := &Identity{Permissions: PermissionSet(tc.perms...)}
			if got := id.Can(tc.check); got != tc.want {
				t.Fatalf("Can(%q) = %v, want %v", tc.check, got, tc.want)
			}
		})
	}
}

func TestSessionPolicySliding(t *testing.T) {
	policy := SessionPolicy{TTL: 10 * time.Hour, MaxLifetime: 100 * time.Hour}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		now       time.Time
		expiresAt time.Time
		wantRenew bool
		wantNext  time.Time
	}{
		{
			name:      "fresh session not renewed",
			now:       created.Add(1 * time.Hour),
			expiresAt: created.Add(10 * time.Hour),
			wantRenew: false,
		},
		{
			name:      "past half TTL renews by full TTL",
			now:       created.Add(6 * time.Hour),
			expiresAt: created.Add(10 * time.Hour),
			wantRenew: true,
			wantNext:  created.Add(16 * time.Hour),
		},
		{
			name:      "renewal clamped at absolute lifetime",
			now:       created.Add(95 * time.Hour),
			expiresAt: created.Add(96 * time.Hour),
			wantRenew: true,
			wantNext:  created.Add(100 * time.Hour),
		},
		{
			name:      "at the cap no further renewal",
			now:       created.Add(99 * time.Hour),
			expiresAt: created.Add(100 * time.Hour),
			wantRenew: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, ok := policy.renewedExpiry(tc.now, tc.expiresAt, created)
			if ok != tc.wantRenew {
				t.Fatalf("renew = %v, want %v", ok, tc.wantRenew)
			}
			if ok && !next.Equal(tc.wantNext) {
				t.Fatalf("next = %v, want %v", next, tc.wantNext)
			}
		})
	}
}

func TestSessionPolicyInitialExpiryCapped(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	policy := SessionPolicy{TTL: 200 * time.Hour, MaxLifetime: 100 * time.Hour}
	if got := policy.InitialExpiry(now); !got.Equal(now.Add(100 * time.Hour)) {
		t.Fatalf("initial expiry %v exceeds max lifetime", got)
	}
}

func TestCookieName(t *testing.T) {
	if CookieName(false) != "session" {
		t.Fatalf("dev cookie = %q", CookieName(false))
	}
	if CookieName(true) != "__Host-session" {
		t.Fatalf("secure cookie = %q, want __Host- prefix", CookieName(true))
	}
}

func TestAuthorize(t *testing.T) {
	rules := map[string]Rule{
		"/test.v1.TestService/Public":    {Public: true},
		"/test.v1.TestService/AuthOnly":  {},
		"/test.v1.TestService/NeedsPerm": {Permission: "user.write"},
	}

	reader := &Identity{UserID: uuid.New(), Permissions: PermissionSet("report.read")}
	writer := &Identity{UserID: uuid.New(), Permissions: PermissionSet("user.write")}

	cases := []struct {
		name      string
		procedure string
		identity  *Identity
		wantCode  connect.Code
	}{
		{"public without session", "/test.v1.TestService/Public", nil, 0},
		{
			"auth-only without session",
			"/test.v1.TestService/AuthOnly",
			nil,
			connect.CodeUnauthenticated,
		},
		{"auth-only with session", "/test.v1.TestService/AuthOnly", reader, 0},
		{
			"permission denied",
			"/test.v1.TestService/NeedsPerm",
			reader,
			connect.CodePermissionDenied,
		},
		{"permission granted", "/test.v1.TestService/NeedsPerm", writer, 0},
		{
			"unlisted procedure denied",
			"/test.v1.TestService/Unknown",
			writer,
			connect.CodePermissionDenied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(rules, tc.procedure, tc.identity)
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if connect.CodeOf(err) != tc.wantCode {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), tc.wantCode)
			}
		})
	}
}
