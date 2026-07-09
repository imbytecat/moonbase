package workflow

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/server/internal/database"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// The sweep reclaims exactly the files that are unattached AND older than the
// cutoff — object and ledger row both vanish — while attached files and files
// within the grace period survive, and a repeat run is a clean no-op (ADR-0003).
func TestReclaimUnattachedSweepsOldOrphansOnly(t *testing.T) {
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

	repo := repository.New(pool)
	dir := t.TempDir()
	store := settings.NewStore(repo)
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []kitsettings.GenericProfile{{
			Id:       "local-sweep",
			Name:     "Local Sweep",
			Provider: "local",
			Config:   map[string]any{"directory": dir},
		}},
		Bindings: map[string][]string{storage.PurposeAvatars: {"local-sweep"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })
	objects := storage.NewClient(store)

	uploader := uuid.New()
	// Both files are created 48h ago so they clear a 24h grace cutoff.
	mkOldFile := func(key string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO files (object_key, content_type, uploaded_by, purpose, created_at)
			 VALUES ($1, 'image/png', $2, 'avatars', now() - interval '48 hours') RETURNING id`,
			key, uploader,
		).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orphanID := mkOldFile("sweep/orphan.png")
	attachedID := mkOldFile("sweep/attached.png")
	if _, err := pool.Exec(ctx,
		`INSERT INTO file_attachments (file_id, owner_type, owner_id) VALUES ($1, 'user', $2)`,
		attachedID, uuid.NewString(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM file_attachments WHERE file_id = $1`, attachedID)
		_, _ = pool.Exec(bg, `DELETE FROM files WHERE id = ANY($1)`, []uuid.UUID{orphanID, attachedID})
	})

	orphanPath := filepath.Join(dir, "sweep", "orphan.png")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan bytes"), 0o640); err != nil {
		t.Fatal(err)
	}

	fileExists := func(id uuid.UUID) bool {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM files WHERE id = $1)`, id).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}

	// Grace period: a cutoff older than the files reclaims nothing.
	n, err := reclaimUnattached(ctx, repo, objects, time.Now().Add(-72*time.Hour), logger)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("with a 72h-ago cutoff, reclaimed = %d, want 0 (files within grace)", n)
	}
	if !fileExists(orphanID) {
		t.Fatal("orphan must survive a sweep whose cutoff predates it")
	}

	// Real sweep: only the unattached, old-enough orphan goes.
	n, err = reclaimUnattached(ctx, repo, objects, time.Now().Add(-24*time.Hour), logger)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reclaimed = %d, want 1 (only the unattached orphan)", n)
	}
	if fileExists(orphanID) {
		t.Fatal("unattached orphan file row must be deleted")
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan object must be deleted from disk, stat err = %v", err)
	}
	if !fileExists(attachedID) {
		t.Fatal("attached file must survive the sweep (its attachment protects it)")
	}

	// Idempotent: a second sweep finds nothing left to reclaim.
	n, err = reclaimUnattached(ctx, repo, objects, time.Now().Add(-24*time.Hour), logger)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second sweep reclaimed = %d, want 0", n)
	}
}

type noopReclaimStore struct{}

func (noopReclaimStore) ListUnattachedFiles(context.Context, time.Time) ([]repository.ListUnattachedFilesRow, error) {
	return nil, nil
}
func (noopReclaimStore) DeleteFile(context.Context, uuid.UUID) error { return nil }

type noopObjectDeleter struct{}

func (noopObjectDeleter) Delete(context.Context, string, string) error { return nil }

// The engine must accept the scheduled reclaim workflow: a bad cron string or a
// signature the DBOS registry rejects would surface here, at New, not silently
// in production.
func TestNewLaunchesWithScheduledReclaim(t *testing.T) {
	dsn := os.Getenv("MOONBASE_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONBASE_DATABASE_URL not set; skipping integration test")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	engine, err := New(context.Background(), dsn, "moonbase", noopReclaimStore{}, noopObjectDeleter{}, logger)
	if err != nil {
		t.Fatalf("engine with the scheduled reclaim workflow must launch: %v", err)
	}
	engine.Shutdown(10 * time.Second)
}
