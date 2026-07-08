package rpc_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/auth/v1/authv1connect"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// End to end over the real wire: replacing an avatar moves the caller's single
// attachment to the new file, leaving the old file with zero attachments
// (unattached, ready for the sweep) — the whole point of ADR-0003's slot
// migration. Exercises the real SetUserAvatar CTE and ownership check.
func TestChangeAvatarTransfersAttachment(t *testing.T) {
	baseURL, client, pool := newStackWithPool(t)
	ctx := t.Context()

	store := settings.NewStore(repository.New(pool))
	if err := store.SetStorage(ctx, settings.Storage{
		Profiles: []systemcodec.StorageProfile{{
			Id:       "local-avatar",
			Name:     "Local Avatar",
			Provider: "local",
			Local:    systemcodec.LocalStorageConfig{Directory: t.TempDir()},
		}},
		Bindings: map[string][]string{storage.PurposeAvatars: {"local-avatar"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.SetStorage(context.Background(), settings.Storage{}) })

	loginAsAdmin(t, baseURL, client)
	authClient := authv1connect.NewAuthServiceClient(client, baseURL)
	storageClient := storagev1connect.NewStorageServiceClient(client, baseURL)

	me, err := authClient.GetMe(ctx, connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	adminID := uuid.MustParse(me.Msg.GetUser().GetId())

	presign := func() string {
		t.Helper()
		resp, err := storageClient.PresignAvatarUpload(ctx, connect.NewRequest(&storagev1.PresignAvatarUploadRequest{
			ContentType:   "image/png",
			ContentLength: 1024,
		}))
		if err != nil {
			t.Fatal(err)
		}
		return resp.Msg.GetFileId()
	}
	attachmentCount := func(fileID string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM file_attachments WHERE file_id = $1`, fileID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	fileA := presign()
	fileB := presign()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `UPDATE users SET avatar_file_id = NULL WHERE id = $1`, adminID)
		_, _ = pool.Exec(bg, `DELETE FROM file_attachments WHERE file_id = ANY($1)`, []string{fileA, fileB})
		_, _ = pool.Exec(bg, `DELETE FROM files WHERE id = ANY($1)`, []string{fileA, fileB})
	})

	if _, err := authClient.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{
		AvatarFileId: proto.String(fileA),
	})); err != nil {
		t.Fatal(err)
	}
	if got := attachmentCount(fileA); got != 1 {
		t.Fatalf("after first save, file A attachments = %d, want 1", got)
	}

	if _, err := authClient.UpdateProfile(ctx, connect.NewRequest(&authv1.UpdateProfileRequest{
		AvatarFileId: proto.String(fileB),
	})); err != nil {
		t.Fatal(err)
	}
	if got := attachmentCount(fileB); got != 1 {
		t.Fatalf("after replacement, file B attachments = %d, want 1", got)
	}
	if got := attachmentCount(fileA); got != 0 {
		t.Fatalf("after replacement, old file A attachments = %d, want 0 (unattached)", got)
	}

	after, err := authClient.GetMe(ctx, connect.NewRequest(&authv1.GetMeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if after.Msg.GetUser().GetAvatarUrl() == "" {
		t.Fatal("GetMe must resolve avatar_url from the new file after a save")
	}
}
