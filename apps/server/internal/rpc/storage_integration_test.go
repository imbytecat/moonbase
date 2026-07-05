package rpc_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// A file referenced by an attachment must be undeletable: the foreign key is
// what makes a dangling reference impossible by construction (ADR-0003).
func TestFileAttachmentBlocksFileDeletion(t *testing.T) {
	_, _, pool := newStackWithPool(t)
	ctx := t.Context()

	var fileID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO files (object_key, content_type, uploaded_by) VALUES ($1, $2, $3) RETURNING id`,
		"avatars/fk-test/abc.png", "image/png", uuid.New(),
	).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM file_attachments WHERE file_id = $1`, fileID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM files WHERE id = $1`, fileID)
	})

	if _, err := pool.Exec(ctx,
		`INSERT INTO file_attachments (file_id, owner_type, owner_id) VALUES ($1, $2, $3)`,
		fileID, "user", uuid.NewString(),
	); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, `DELETE FROM files WHERE id = $1`, fileID)
	if err == nil {
		t.Fatal("deleting a referenced file must fail on the foreign key, but it succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("want foreign_key_violation (23503), got %v", err)
	}
}

// End to end over the real wire: presigning an avatar upload lands a files row
// describing exactly the object it authorized (ADR-0003 "presign 即落库"). This
// exercises the real InsertFile SQL, migration, and storage resolution the
// unit test fakes out.
func TestPresignAvatarUploadLandsFilesRow(t *testing.T) {
	baseURL, client, pool := newStackWithPool(t)
	ctx := t.Context()

	// Bind the avatars purpose to a local storage profile so presign issues a
	// signed URL without any external backend.
	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []systemcodec.StorageProfile{{
			Id:       "local-test",
			Name:     "Local Test",
			Provider: "local",
			Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
		}},
		Bindings: map[string][]string{storage.PurposeAvatars: {"local-test"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)
	me, err := authv1connect.NewAuthServiceClient(client, baseURL).
		GetMe(ctx, connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	adminID := me.Msg.GetUser().GetId()

	resp, err := storagev1connect.NewStorageServiceClient(client, baseURL).
		PresignAvatarUpload(ctx, connect.NewRequest(&storagev1.PresignAvatarUploadRequest{
			ContentType:   "image/png",
			ContentLength: 2048,
		}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetFileId() == "" {
		t.Fatal("presign response must carry a file_id")
	}
	fileID, err := uuid.Parse(resp.Msg.GetFileId())
	if err != nil {
		t.Fatalf("file_id %q is not a uuid: %v", resp.Msg.GetFileId(), err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM files WHERE id = $1`, fileID)
	})

	var (
		objectKey   string
		contentType string
		uploadedBy  uuid.UUID
	)
	if err := pool.QueryRow(ctx,
		`SELECT object_key, content_type, uploaded_by FROM files WHERE id = $1`, fileID,
	).Scan(&objectKey, &contentType, &uploadedBy); err != nil {
		t.Fatalf("no files row for file_id %s: %v", fileID, err)
	}
	if objectKey != resp.Msg.GetObjectKey() {
		t.Fatalf("persisted object_key = %q, response object_key = %q", objectKey, resp.Msg.GetObjectKey())
	}
	if contentType != "image/png" {
		t.Fatalf("persisted content_type = %q, want image/png", contentType)
	}
	if uploadedBy.String() != adminID {
		t.Fatalf("persisted uploaded_by = %s, want admin %s", uploadedBy, adminID)
	}
}
