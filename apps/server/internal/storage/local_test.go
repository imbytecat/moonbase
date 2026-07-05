package storage

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

type memQuerier struct {
	repository.Querier
	rows map[string][]byte
}

func (m *memQuerier) GetSetting(_ context.Context, key string) (repository.Setting, error) {
	raw, ok := m.rows[key]
	if !ok {
		return repository.Setting{}, pgx.ErrNoRows
	}
	return repository.Setting{Key: key, Value: raw}, nil
}

func (m *memQuerier) UpsertSetting(_ context.Context, arg repository.UpsertSettingParams) error {
	m.rows[arg.Key] = arg.Value
	return nil
}

func newLocalFixture(t *testing.T) (*settings.Store, *Client) {
	t.Helper()
	store := settings.NewStore(&memQuerier{rows: map[string][]byte{}})
	cfg := settings.Storage{
		Profiles: []systemcodec.StorageProfile{{
			Id:       "p1",
			Name:     "local",
			Provider: "local",
			Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
		}},
		Bindings: map[string][]string{PurposeAvatars: {"p1"}},
	}
	if err := store.SetStorage(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	return store, NewClient(store)
}

func TestLocalPutThenGetRoundtrip(t *testing.T) {
	store, client := newLocalFixture(t)
	mux := http.NewServeMux()
	mux.Handle("/files/{purpose}/{key...}", NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv := httptest.NewServer(http.StripPrefix("/api", mux))
	defer srv.Close()

	putURL, err := client.PresignPut(t.Context(), PurposeAvatars, "u1/pic.png", "image/png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+putURL, strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", res.StatusCode)
	}

	getURL, err := client.ResolveURL(t.Context(), PurposeAvatars, "u1/pic.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.Get(srv.URL + getURL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "payload" {
		t.Fatalf("GET = %d %q, want 200 \"payload\"", res.StatusCode, body)
	}
}

func TestLocalRejectsTamperedAndExpiredSignatures(t *testing.T) {
	store, client := newLocalFixture(t)
	handler := NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	mux.Handle("/files/{purpose}/{key...}", handler)

	signed, err := client.ResolveURL(t.Context(), PurposeAvatars, "u1/pic.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}

	tampered := *u
	q := tampered.Query()
	q.Set("sig", strings.Repeat("0", 64))
	tampered.RawQuery = q.Encode()

	expired := *u
	q = expired.Query()
	q.Set("exp", strconv.FormatInt(0, 10))
	expired.RawQuery = q.Encode()

	wrongMethod := *u

	for name, tc := range map[string]struct {
		method string
		target string
	}{
		"tampered signature": {http.MethodGet, tampered.String()},
		"expired signature":  {http.MethodGet, expired.String()},
		"method mismatch":    {http.MethodPut, wrongMethod.String()},
	} {
		req := httptest.NewRequest(tc.method, strings.TrimPrefix(tc.target, "/api"), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, rec.Code)
		}
	}
}

func TestLocalObjectPathRejectsTraversal(t *testing.T) {
	cfg := systemcodec.LocalStorageConfig{Directory: "/srv/data"}
	if _, err := localObjectPath(cfg, "../etc/passwd"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := localObjectPath(cfg, "avatars/u1/pic.png"); err != nil {
		t.Fatalf("legit key rejected: %v", err)
	}
}

func TestLocalTestProvesDirectoryWritable(t *testing.T) {
	_, client := newLocalFixture(t)
	err := client.TestConnection(t.Context(), systemcodec.StorageProfile{
		Provider: "local",
		Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("writable directory must pass: %v", err)
	}
}
