package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	cases := []struct {
		name            string
		pingErr         error
		wantStatus      int
		wantStatusField string
		wantDB          string
	}{
		{"database up", nil, 200, "ok", "up"},
		{"database down", errors.New("connection refused"), 503, "degraded", "down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealthHandler(fakePinger{err: tc.pingErr})
			rec := httptest.NewRecorder()
			h.Health(rec, httptest.NewRequest("GET", "/health", nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("status code = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if body["status"] != tc.wantStatusField {
				t.Errorf("status = %q, want %q", body["status"], tc.wantStatusField)
			}
			if body["database"] != tc.wantDB {
				t.Errorf("database = %q, want %q", body["database"], tc.wantDB)
			}
			if body["version"] == "" {
				t.Error("version field missing from health payload")
			}
		})
	}
}
