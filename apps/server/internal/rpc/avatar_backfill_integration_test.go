package rpc_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/imbytecat/moonbase/server/db"
)

// Version of the files.purpose migration — the last one before avatars move off
// the raw key. Seeding the legacy shape means migrating up to here, no further.
const migrationBeforeAvatarFileID = 20260715000000

// The avatar migration must carry existing avatars across, not drop them: for
// every user still holding a raw avatar_key it mints a files row (purpose
// avatars, content type inferred from the extension) plus an attachment, and
// repoints the user (ADR-0003 存量回填). Verified against a throwaway database
// seeded in the pre-migration shape, then migrated the rest of the way.
func TestAvatarBackfillMigration(t *testing.T) {
	baseDSN := os.Getenv("MOONBASE_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("MOONBASE_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	dbName := "moonbase_migtest_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	})

	sqlDB, err := sql.Open("pgx", swapDBName(t, baseDSN, dbName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.Migrations())
	if err != nil {
		t.Fatal(err)
	}

	// Migrate to the pre-avatar-migration schema, where users still carry a raw
	// avatar_key column, and seed a legacy avatar.
	if _, err := provider.UpTo(ctx, migrationBeforeAvatarFileID); err != nil {
		t.Fatal(err)
	}
	const legacyKey = "avatars/legacy-user/pic.png"
	var userID uuid.UUID
	if err := sqlDB.QueryRowContext(
		ctx,
		`INSERT INTO users (email, name, password_hash, avatar_key) VALUES ($1,$2,$3,$4) RETURNING id`,
		"legacy@example.com",
		"Legacy",
		"hash",
		legacyKey,
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	// Apply the avatar migration: this is the backfill under test.
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var (
		fileID      uuid.UUID
		objectKey   string
		contentType string
		purpose     string
		uploadedBy  uuid.UUID
	)
	if err := sqlDB.QueryRowContext(
		ctx,
		`SELECT id, object_key, content_type, purpose, uploaded_by FROM files WHERE object_key = $1`,
		legacyKey,
	).Scan(&fileID, &objectKey, &contentType, &purpose, &uploadedBy); err != nil {
		t.Fatalf("no files row backfilled for legacy avatar: %v", err)
	}
	if purpose != "avatars" {
		t.Fatalf("backfilled purpose = %q, want avatars", purpose)
	}
	if contentType != "image/png" {
		t.Fatalf("backfilled content_type = %q, want image/png (inferred from .png)", contentType)
	}
	if uploadedBy != userID {
		t.Fatalf("backfilled uploaded_by = %s, want owner %s", uploadedBy, userID)
	}

	var attachments int
	if err := sqlDB.QueryRowContext(
		ctx,
		`SELECT count(*) FROM file_attachments WHERE file_id = $1 AND owner_type = 'user' AND owner_id = $2`,
		fileID,
		userID.String(),
	).Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments != 1 {
		t.Fatalf("backfilled attachments = %d, want 1 keeping the file alive", attachments)
	}

	var avatarFileID uuid.UUID
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT avatar_file_id FROM users WHERE id = $1`, userID,
	).Scan(&avatarFileID); err != nil {
		t.Fatal(err)
	}
	if avatarFileID != fileID {
		t.Fatalf("user avatar_file_id = %s, want backfilled file %s", avatarFileID, fileID)
	}
}

func swapDBName(t *testing.T, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}
