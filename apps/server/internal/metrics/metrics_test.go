package metrics

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakePool struct{}

func (fakePool) Stat() PoolStat { return PoolStat{Acquired: 3, Idle: 7, Total: 10, Max: 20} }

func TestCodeString(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, "ok"},
		{"typed", connect.NewError(connect.CodeNotFound, errors.New("x")), "not_found"},
		{
			"denied",
			connect.NewError(connect.CodePermissionDenied, errors.New("x")),
			"permission_denied",
		},
		{"plain", errors.New("bare"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := codeString(tc.err); got != tc.want {
				t.Fatalf("codeString(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestInterceptorAndScrape drives one successful and one failed RPC through the
// interceptor, then scrapes /metrics and asserts the exposition carries the RPC
// series (both result codes), the DB pool gauges, and build info — i.e. the
// whole pipeline from instrument to text format works end to end.
func TestInterceptorAndScrape(t *testing.T) {
	m := New(fakePool{})
	intercept := m.Interceptor()

	ok := intercept(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&emptypb.Empty{}), nil
	})
	fail := intercept(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("nope"))
	})

	req := connect.NewRequest(&emptypb.Empty{})
	if _, err := ok(context.Background(), req); err != nil {
		t.Fatalf("ok RPC returned error: %v", err)
	}
	if _, err := fail(context.Background(), req); err == nil {
		t.Fatal("fail RPC returned nil error")
	}

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("scrape status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{
		`moonbase_rpc_requests_total{code="ok"`,
		`moonbase_rpc_requests_total{code="not_found"`,
		"moonbase_rpc_request_duration_seconds_bucket",
		"moonbase_rpc_in_flight_requests",
		"moonbase_db_connections_max 20",
		"moonbase_db_connections_acquired 3",
		"moonbase_build_info",
		"go_goroutines", // Go runtime collector present
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape missing %q", want)
		}
	}
}

// TestNewNilPoolOmitsDBGauges pins that a nil pool (unit tests / no DB) is safe
// and simply drops the connection gauges rather than panicking.
func TestNewNilPoolOmitsDBGauges(t *testing.T) {
	m := New(nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(rec.Body.String(), "moonbase_db_connections_max") {
		t.Error("nil pool should omit DB gauges")
	}
}
