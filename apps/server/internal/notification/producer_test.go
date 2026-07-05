package notification

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/i18n"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

// fakeRepo embeds Querier so only the three methods the producer touches are
// implemented; anything else would panic, proving they are never called.
type fakeRepo struct {
	repository.Querier
	locales  map[uuid.UUID]string
	byPerm   map[string][]repository.ListUserLocalesForPermissionRow
	inserted []repository.InsertNotificationParams
}

func (f *fakeRepo) GetUserLocale(_ context.Context, id uuid.UUID) (string, error) {
	return f.locales[id], nil
}

func (f *fakeRepo) ListUserLocalesForPermission(_ context.Context, perm string) ([]repository.ListUserLocalesForPermissionRow, error) {
	return f.byPerm[perm], nil
}

func (f *fakeRepo) InsertNotification(_ context.Context, arg repository.InsertNotificationParams) error {
	f.inserted = append(f.inserted, arg)
	return nil
}

func TestPublishRendersInRecipientLocale(t *testing.T) {
	u := uuid.New()
	f := &fakeRepo{locales: map[uuid.UUID]string{u: "en"}}
	if err := NewProducer(f).Publish(context.Background(), u, Message{
		Category: CategorySystem,
		TitleKey: i18n.AuthCodeSubject,
	}); err != nil {
		t.Fatal(err)
	}
	if len(f.inserted) != 1 || f.inserted[0].Title != "Your verification code" {
		t.Fatalf("inserted = %+v, want one en-titled row", f.inserted)
	}
}

func TestPublishToPermissionLocalizesPerRecipient(t *testing.T) {
	zh, en := uuid.New(), uuid.New()
	f := &fakeRepo{byPerm: map[string][]repository.ListUserLocalesForPermissionRow{
		"system.read": {{ID: zh, Locale: "zh-CN"}, {ID: en, Locale: "en"}},
	}}
	if err := NewProducer(f).PublishToPermission(context.Background(), "system.read", Message{
		Category: CategorySystem,
		TitleKey: i18n.AuthCodeSubject,
	}); err != nil {
		t.Fatal(err)
	}
	titles := map[uuid.UUID]string{}
	for _, ins := range f.inserted {
		titles[ins.UserID] = ins.Title
	}
	if titles[zh] != "你的验证码" || titles[en] != "Your verification code" {
		t.Fatalf("per-recipient titles = %v", titles)
	}
}
