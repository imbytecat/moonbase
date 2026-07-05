// Package notification is the producer side of the in-app inbox (站内信): the
// domain that owns an event calls Publish to drop a message into a user's
// inbox. Title/body are stored already-rendered, so the producer localizes to
// EACH recipient's own language at publish time (a fan-out to admins who
// differ in locale gets each their own language). Reading the inbox lives in
// internal/rpc; this half only writes.
package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/i18n"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

// Notification categories — code-defined slots the frontend maps to an icon
// and label (mirrored in src/lib/notifications.ts).
const (
	CategorySystem   = "system"
	CategorySecurity = "security"
	CategoryAccount  = "account"
	CategoryPayment  = "payment"
	CategoryWorkflow = "workflow"
)

// Message is a localizable notification: a category, an optional in-app deep
// link, and i18n keys (+ args) for title and body. The producer renders the
// keys in each recipient's locale, so callers pass keys, never rendered text.
type Message struct {
	Category  string
	Link      string
	TitleKey  string
	TitleArgs []any
	BodyKey   string
	BodyArgs  []any
}

// Publisher delivers a notification to one user, or fans it out to everyone
// holding a permission. Adding a feature that notifies = calling one of these
// from the domain that owns the event.
type Publisher interface {
	Publish(ctx context.Context, userID uuid.UUID, msg Message) error
	PublishToPermission(ctx context.Context, permission string, msg Message) error
}

type Producer struct {
	repo repository.Querier
}

func NewProducer(repo repository.Querier) *Producer {
	return &Producer{repo: repo}
}

var _ Publisher = (*Producer)(nil)

func (p *Producer) Publish(ctx context.Context, userID uuid.UUID, msg Message) error {
	locale, err := p.repo.GetUserLocale(ctx, userID)
	if err != nil {
		return fmt.Errorf("notification recipient locale: %w", err)
	}
	return p.insert(ctx, userID, locale, msg)
}

func (p *Producer) PublishToPermission(ctx context.Context, permission string, msg Message) error {
	recipients, err := p.repo.ListUserLocalesForPermission(ctx, permission)
	if err != nil {
		return fmt.Errorf("notification recipients for %q: %w", permission, err)
	}
	// Best-effort per recipient: one failed insert must not drop the others.
	var errs []error
	for _, r := range recipients {
		if err := p.insert(ctx, r.ID, r.Locale, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Producer) insert(ctx context.Context, userID uuid.UUID, storedLocale string, msg Message) error {
	loc := i18n.Resolve(storedLocale, "")
	return p.repo.InsertNotification(ctx, repository.InsertNotificationParams{
		UserID:   userID,
		Category: msg.Category,
		Title:    i18n.T(loc, msg.TitleKey, msg.TitleArgs...),
		Body:     i18n.T(loc, msg.BodyKey, msg.BodyArgs...),
		Link:     msg.Link,
	})
}
