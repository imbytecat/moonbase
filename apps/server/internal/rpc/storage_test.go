package rpc

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/auth"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

// stubObjectStore issues a fixed upload URL so presign tests observe the
// ledger behavior (a files row is written) without a real backend.
type stubObjectStore struct {
	url string
}

func (s stubObjectStore) PresignPut(context.Context, string, string, string, time.Duration) (string, error) {
	return s.url, nil
}

func (stubObjectStore) ResolveURL(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

type fakeStorageQuerier struct {
	repository.Querier
	insertFile func(ctx context.Context, arg repository.InsertFileParams) (repository.File, error)
}

func (f *fakeStorageQuerier) InsertFile(ctx context.Context, arg repository.InsertFileParams) (repository.File, error) {
	return f.insertFile(ctx, arg)
}

// Presigning an avatar upload must, in the same call, write a files row the
// system owns from the first second (ADR-0003 "presign 即落库") — recording the
// server-picked key, the declared content type, and the caller as uploader —
// and hand its id back as file_id alongside the object_key.
func TestPresignAvatarUploadPersistsFile(t *testing.T) {
	userID := uuid.New()
	fileID := uuid.New()

	var got repository.InsertFileParams
	repo := &fakeStorageQuerier{
		insertFile: func(_ context.Context, arg repository.InsertFileParams) (repository.File, error) {
			got = arg
			return repository.File{
				ID:          fileID,
				ObjectKey:   arg.ObjectKey,
				ContentType: arg.ContentType,
				UploadedBy:  arg.UploadedBy,
			}, nil
		},
	}
	svc := NewStorageService(repo, stubObjectStore{url: "https://storage.test/put"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})
	resp, err := svc.PresignAvatarUpload(ctx, connect.NewRequest(&storagev1.PresignAvatarUploadRequest{
		ContentType:   "image/png",
		ContentLength: 1024,
	}))
	if err != nil {
		t.Fatal(err)
	}

	// The persisted row must describe exactly the object the response authorizes.
	if got.ObjectKey != resp.Msg.GetObjectKey() {
		t.Fatalf("persisted object_key = %q, response object_key = %q; must match",
			got.ObjectKey, resp.Msg.GetObjectKey())
	}
	if got.ContentType != "image/png" {
		t.Fatalf("persisted content_type = %q, want image/png", got.ContentType)
	}
	if got.UploadedBy != userID {
		t.Fatalf("persisted uploaded_by = %v, want caller %v", got.UploadedBy, userID)
	}
	// The row records the storage purpose so the unattached sweep can later
	// resolve which backend to delete the object from (ADR-0003).
	if got.Purpose != "avatars" {
		t.Fatalf("persisted purpose = %q, want avatars", got.Purpose)
	}
	// file_id echoes the row's id so consumers can attach it later.
	if resp.Msg.GetFileId() != fileID.String() {
		t.Fatalf("response file_id = %q, want persisted id %q", resp.Msg.GetFileId(), fileID.String())
	}
}

// Site-asset (logo / favicon) presign follows the same ledger rule: one files
// row per authorized object, its id returned as file_id.
func TestPresignSiteAssetUploadPersistsFile(t *testing.T) {
	userID := uuid.New()
	fileID := uuid.New()

	var got repository.InsertFileParams
	repo := &fakeStorageQuerier{
		insertFile: func(_ context.Context, arg repository.InsertFileParams) (repository.File, error) {
			got = arg
			return repository.File{
				ID:          fileID,
				ObjectKey:   arg.ObjectKey,
				ContentType: arg.ContentType,
				UploadedBy:  arg.UploadedBy,
			}, nil
		},
	}
	svc := NewStorageService(repo, stubObjectStore{url: "https://storage.test/put"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})
	resp, err := svc.PresignSiteAssetUpload(ctx, connect.NewRequest(&storagev1.PresignSiteAssetUploadRequest{
		Kind:          "logo",
		ContentType:   "image/png",
		ContentLength: 1024,
	}))
	if err != nil {
		t.Fatal(err)
	}

	if got.ObjectKey != resp.Msg.GetObjectKey() {
		t.Fatalf("persisted object_key = %q, response object_key = %q; must match",
			got.ObjectKey, resp.Msg.GetObjectKey())
	}
	if got.ContentType != "image/png" {
		t.Fatalf("persisted content_type = %q, want image/png", got.ContentType)
	}
	if got.UploadedBy != userID {
		t.Fatalf("persisted uploaded_by = %v, want caller %v", got.UploadedBy, userID)
	}
	if got.Purpose != "site-assets" {
		t.Fatalf("persisted purpose = %q, want site-assets", got.Purpose)
	}
	if resp.Msg.GetFileId() != fileID.String() {
		t.Fatalf("response file_id = %q, want persisted id %q", resp.Msg.GetFileId(), fileID.String())
	}
}
