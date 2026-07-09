package rpc_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/server/integrationkit/settings"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// End to end over the real wire (ADR-0004 last cell): a private-purpose file
// behind /f/{file_id} answers 401 to anonymous callers without redirecting,
// and 302 with Cache-Control: private, no-store to a session-authenticated
// caller; following the short-lived signed Location returns the bytes. No
// real private purpose exists yet, so an unknown purpose (fail-closed to
// private) proves the path for the first real one.
func TestPermanentFileURLPrivatePurposeAuthThenSignedRedirect(t *testing.T) {
	const privatePurpose = "private-test"
	baseURL, client, pool := newStackWithPool(t)
	serverURL := strings.TrimSuffix(baseURL, "/api")
	ctx := t.Context()

	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []kitsettings.GenericProfile{{
			Id:       "local-priv",
			Name:     "Local Private",
			Provider: "local",
			Config:   map[string]any{"directory": t.TempDir()},
		}},
		Bindings: map[string][]string{privatePurpose: {"local-priv"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)

	var fileID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO files (object_key, content_type, uploaded_by, purpose) VALUES ($1, $2, $3, $4) RETURNING id`,
		"m1/doc.txt", "text/plain", uuid.New(), privatePurpose,
	).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM files WHERE id = $1`, fileID)
	})

	objects := storage.NewClient(store)
	putURL, err := objects.PresignPut(ctx, privatePurpose, "m1/doc.txt", "text/plain", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, serverURL+putURL, strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}
	putRes, err := client.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("signed PUT status = %d, want 200", putRes.StatusCode)
	}

	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := anon.Get(serverURL + "/f/" + fileID.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous GET /f private = %d, want 401", res.StatusCode)
	}
	if got := res.Header.Get("Location"); got != "" {
		t.Fatalf("anonymous must not redirect, got Location %q", got)
	}

	authed := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err = authed.Get(serverURL + "/f/" + fileID.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("authenticated GET /f private = %d, want 302", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "sig=") || !strings.Contains(loc, "exp=") {
		t.Fatalf("Location = %q, want a short-lived signed URL", loc)
	}

	fetched, err := anon.Get(serverURL + loc)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(fetched.Body)
	_ = fetched.Body.Close()
	if fetched.StatusCode != http.StatusOK || string(body) != "secret" {
		t.Fatalf("signed GET = %d %q, want 200 \"secret\"", fetched.StatusCode, body)
	}
}

// Same private path over the S3 driver: authenticated /f/ answers a
// no-store 302 whose Location is a presigned S3 GET that returns the bytes.
// Skips without MOONBASE_TEST_S3_ENDPOINT (compose.yaml's SeaweedFS:
// localhost:8333, bucket app, keys seaweedadmin/seaweedadmin).
func TestPermanentFileURLPrivatePurposeS3SignedRedirect(t *testing.T) {
	endpoint := os.Getenv("MOONBASE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("MOONBASE_TEST_S3_ENDPOINT not set; skipping S3 integration test")
	}
	const privatePurpose = "private-test"
	baseURL, client, pool := newStackWithPool(t)
	serverURL := strings.TrimSuffix(baseURL, "/api")
	ctx := t.Context()

	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []kitsettings.GenericProfile{{
			Id:       "s3-priv",
			Name:     "S3 Private",
			Provider: "s3",
			Config: map[string]any{
				"endpoint":        endpoint,
				"region":          "us-east-1",
				"bucket":          "app",
				"accessKeyId":     "seaweedadmin",
				"secretAccessKey": "seaweedadmin",
			},
		}},
		Bindings: map[string][]string{privatePurpose: {"s3-priv"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)

	key := "private-test/" + uuid.NewString() + ".txt"
	var fileID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO files (object_key, content_type, uploaded_by, purpose) VALUES ($1, $2, $3, $4) RETURNING id`,
		key, "text/plain", uuid.New(), privatePurpose,
	).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM files WHERE id = $1`, fileID)
	})

	objects := storage.NewClient(store)
	putURL, err := objects.PresignPut(ctx, privatePurpose, key, "text/plain", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}
	putReq.Header.Set("Content-Type", "text/plain")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = putRes.Body.Close()
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("presigned PUT status = %d, want 200", putRes.StatusCode)
	}
	t.Cleanup(func() {
		_ = objects.Delete(context.Background(), privatePurpose, key)
	})

	authed := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := authed.Get(serverURL + "/f/" + fileID.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("authenticated GET /f private(s3) = %d, want 302", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}

	fetched, err := http.Get(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(fetched.Body)
	_ = fetched.Body.Close()
	if fetched.StatusCode != http.StatusOK || string(body) != "secret" {
		t.Fatalf("presigned GET = %d %q, want 200 \"secret\"", fetched.StatusCode, body)
	}
}
