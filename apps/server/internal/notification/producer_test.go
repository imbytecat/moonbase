package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// fakeRepo embeds Querier so only the two methods the producer touches are
// implemented; anything else would panic, proving they are never called.
type fakeRepo struct {
	repository.Querier
	byPerm   map[string][]uuid.UUID
	inserted []repository.InsertNotificationParams
}

func (f *fakeRepo) ListUsersForPermission(_ context.Context, perm string) ([]uuid.UUID, error) {
	return f.byPerm[perm], nil
}

func (f *fakeRepo) InsertNotification(_ context.Context, arg repository.InsertNotificationParams) error {
	f.inserted = append(f.inserted, arg)
	return nil
}

func TestPublishStoresMessage(t *testing.T) {
	u := uuid.New()
	f := &fakeRepo{}
	if err := NewProducer(f).Publish(context.Background(), u, Message{
		Category: CategorySystem,
		Title:    "你的验证码",
	}); err != nil {
		t.Fatal(err)
	}
	if len(f.inserted) != 1 || f.inserted[0].Title != "你的验证码" {
		t.Fatalf("inserted = %+v, want one titled row", f.inserted)
	}
}

func TestPublishToPermissionFansOut(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	f := &fakeRepo{byPerm: map[string][]uuid.UUID{
		"system.read": {a, b},
	}}
	if err := NewProducer(f).PublishToPermission(context.Background(), "system.read", Message{
		Category: CategorySystem,
		Title:    "你的验证码",
	}); err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]string{}
	for _, ins := range f.inserted {
		got[ins.UserID] = ins.Title
	}
	if got[a] != "你的验证码" || got[b] != "你的验证码" {
		t.Fatalf("fan-out titles = %v", got)
	}
}
