package storage

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	kitsettings "github.com/imbytecat/moonbase/server/integrationkit/settings"
	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

type fileQuerier struct {
	repository.Querier
	files map[uuid.UUID]repository.File
}

func (f *fileQuerier) GetFile(_ context.Context, id uuid.UUID) (repository.File, error) {
	file, ok := f.files[id]
	if !ok {
		return repository.File{}, pgx.ErrNoRows
	}
	return file, nil
}

func newFileHandlerFixture(t *testing.T, files map[uuid.UUID]repository.File) (*settings.Store, *httptest.Server, string) {
	t.Helper()
	store, client := newLocalFixture(t)
	mux := http.NewServeMux()
	mux.Handle("GET /f/{file_id}", NewFileHandler(store, client, &fileQuerier{files: files},
		slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	st, err := store.Storage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := st.ProfileFor(PurposeAvatars)
	return store, srv, cfgStr(cfg.Config, "directory")
}

func TestFileHandlerServesPublicLocalFile(t *testing.T) {
	fileID := uuid.New()
	files := map[uuid.UUID]repository.File{
		fileID: {ID: fileID, ObjectKey: "u1/pic.png", ContentType: "image/png", Purpose: PurposeAvatars},
	}
	_, srv, dir := newFileHandlerFixture(t, files)

	if err := os.MkdirAll(filepath.Join(dir, "u1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "u1", "pic.png"), []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/f/" + fileID.String())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "payload" {
		t.Fatalf("GET /f = %d %q, want 200 \"payload\"", res.StatusCode, body)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want public immutable year", got)
	}
	if got := res.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
}

func TestFileHandlerUnknownOrInvalidIDReturns404(t *testing.T) {
	_, srv, _ := newFileHandlerFixture(t, map[uuid.UUID]repository.File{})

	for name, id := range map[string]string{
		"unknown uuid": uuid.NewString(),
		"not a uuid":   "not-a-uuid",
	} {
		res, err := http.Get(srv.URL + "/f/" + id)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, res.StatusCode)
		}
	}
}

// private × any driver (ADR-0004 last cell): anonymous → 401 with no
// redirect; authenticated → 302 to the driver's short-lived signed URL with
// Cache-Control: private, no-store — the redirect target carries an expiry,
// so caching the 302 would bypass the auth window.
func TestFileHandlerPrivatePurposeRequiresAuthThenRedirectsToSignedURL(t *testing.T) {
	const privatePurpose = "private-test"
	fileID := uuid.New()
	files := map[uuid.UUID]repository.File{
		fileID: {ID: fileID, ObjectKey: "m1/doc.pdf", ContentType: "application/pdf", Purpose: privatePurpose},
	}

	store, client := newLocalFixture(t)
	st, err := store.Storage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	st.Bindings[privatePurpose] = []string{"p1"}
	if err := store.SetStorage(t.Context(), st); err != nil {
		t.Fatal(err)
	}

	handler := NewFileHandler(store, client, &fileQuerier{files: files},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	mux.Handle("GET /f/{file_id}", handler)

	anon := httptest.NewRequest(http.MethodGet, "/f/"+fileID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, anon)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("anonymous must not redirect, got Location %q", got)
	}

	authed := anon.Clone(auth.WithIdentity(t.Context(), &auth.Identity{UserID: uuid.New()}))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authed)
	if rec.Code != http.StatusFound {
		t.Fatalf("authenticated status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "sig=") || !strings.Contains(loc, "exp=") {
		t.Fatalf("Location = %q, want a short-lived signed URL", loc)
	}
}

// public × S3 with a public base URL: redirect to the stable public URL. The
// short cache (1h, not a year) bounds how long a stale redirect survives a
// bucket rebinding (ADR-0004).
func TestFileHandlerRedirectsPublicS3ToStableURL(t *testing.T) {
	fileID := uuid.New()
	files := &fileQuerier{files: map[uuid.UUID]repository.File{
		fileID: {ID: fileID, ObjectKey: "u1/pic.png", ContentType: "image/png", Purpose: PurposeAvatars},
	}}

	store := settings.NewStore(&memQuerier{rows: map[string][]byte{}})
	if err := store.SetStorage(t.Context(), settings.Storage{
		Profiles: []kitsettings.GenericProfile{{
			Id:       "s3",
			Name:     "s3",
			Provider: "s3",
			Config: map[string]any{
				"endpoint":      "s3.test",
				"bucket":        "b",
				"accessKeyId":   "k",
				"publicBaseUrl": "https://cdn.test",
			},
		}},
		Bindings: map[string][]string{PurposeAvatars: {"s3"}},
	}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /f/{file_id}", NewFileHandler(store, NewClient(store), files,
		slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(srv.URL + "/f/" + fileID.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if got := res.Header.Get("Location"); got != "https://cdn.test/u1/pic.png" {
		t.Fatalf("Location = %q, want stable public URL", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public max-age=3600", got)
	}
}
