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

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
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
		Profiles: []kitsettings.GenericProfile{{
			Id:       "p1",
			Name:     "local",
			Provider: "local",
			Config:   map[string]any{"directory": t.TempDir()},
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

// Visibility is a static property of the purpose (CONTEXT.md): public
// purposes (avatars, site assets) are readable without a signature and served
// with an immutable year-long cache header — files are spiritually immutable
// (ADR-0003), so the aggressive cache is sound.
func TestLocalServesPublicPurposeWithoutSignature(t *testing.T) {
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

	res, err = http.Get(srv.URL + "/api/files/" + PurposeAvatars + "/u1/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "payload" {
		t.Fatalf("unsigned GET public = %d %q, want 200 \"payload\"", res.StatusCode, body)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want public immutable year", got)
	}
}

// Signature rejection is exercised through a private purpose (unknown
// purposes fail closed to private): public GETs skip the signature entirely,
// but private GETs and every PUT still require a valid one.
func TestLocalRejectsTamperedAndExpiredSignatures(t *testing.T) {
	const privatePurpose = "private-test"
	store, client := newLocalFixture(t)
	handler := NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	mux.Handle("/files/{purpose}/{key...}", handler)

	signed, err := client.LocalSignedURL(t.Context(), http.MethodGet, privatePurpose, "u1/pic.png", time.Minute)
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
		"tampered signature":   {http.MethodGet, tampered.String()},
		"expired signature":    {http.MethodGet, expired.String()},
		"method mismatch":      {http.MethodPut, wrongMethod.String()},
		"unsigned private GET": {http.MethodGet, "/files/" + privatePurpose + "/u1/pic.png"},
		"unsigned public PUT":  {http.MethodPut, "/files/" + PurposeAvatars + "/u1/pic.png"},
	} {
		req := httptest.NewRequest(tc.method, strings.TrimPrefix(tc.target, "/api"), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, rec.Code)
		}
	}
}

func TestLocalResolveURLPublicPurposeIsUnsignedAndStable(t *testing.T) {
	_, client := newLocalFixture(t)
	got, err := client.ResolveURL(t.Context(), PurposeAvatars, "u1/pic.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/api/files/" + PurposeAvatars + "/u1/pic.png"; got != want {
		t.Fatalf("ResolveURL(public) = %q, want unsigned stable %q", got, want)
	}
}

func TestLocalObjectPathRejectsTraversal(t *testing.T) {
	cfg := map[string]any{"directory": "/srv/data"}
	if _, err := storageint.LocalObjectPath(cfg, "../etc/passwd"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := storageint.LocalObjectPath(cfg, "avatars/u1/pic.png"); err != nil {
		t.Fatalf("legit key rejected: %v", err)
	}
}

func TestLocalTestProvesDirectoryWritable(t *testing.T) {
	_, client := newLocalFixture(t)
	err := client.TestConnection(t.Context(), kitsettings.GenericProfile{
		Provider: "local",
		Config:   map[string]any{"directory": t.TempDir()},
	})
	if err != nil {
		t.Fatalf("writable directory must pass: %v", err)
	}
}

// The unattached-file sweep deletes objects through ObjectStore.Delete, so the
// local driver must remove the on-disk file — and deleting a key that is
// already gone must be a no-op, not an error, so a crash-resumed sweep can
// re-run safely (ADR-0003 idempotent Delete).
func TestLocalDeleteRemovesObjectAndIsIdempotent(t *testing.T) {
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

	if err := client.Delete(t.Context(), PurposeAvatars, "u1/pic.png"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	getURL, err := client.ResolveURL(t.Context(), PurposeAvatars, "u1/pic.png", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.Get(srv.URL + getURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete, GET status = %d, want 404", res.StatusCode)
	}

	if err := client.Delete(t.Context(), PurposeAvatars, "u1/pic.png"); err != nil {
		t.Fatalf("deleting an already-gone object must be a no-op, got %v", err)
	}
}
