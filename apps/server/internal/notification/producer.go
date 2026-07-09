// Package notification is the producer side of the in-app inbox (站内信): the
// domain that owns an event calls Publish to drop a message into a user's
// inbox. The whole app is Simplified Chinese, so callers pass finished title
// and body text and the producer stores it verbatim. Reading the inbox lives
// in internal/rpc; this half only writes.
package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

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

// Message is a notification: a category, an optional in-app deep link, and the
// already-rendered title and body.
type Message struct {
	Category string
	Link     string
	Title    string
	Body     string
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
	return p.insert(ctx, userID, msg)
}

func (p *Producer) PublishToPermission(ctx context.Context, permission string, msg Message) error {
	recipients, err := p.repo.ListUsersForPermission(ctx, permission)
	if err != nil {
		return fmt.Errorf("notification recipients for %q: %w", permission, err)
	}
	// Best-effort per recipient: one failed insert must not drop the others.
	var errs []error
	for _, userID := range recipients {
		if err := p.insert(ctx, userID, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Producer) insert(ctx context.Context, userID uuid.UUID, msg Message) error {
	return p.repo.InsertNotification(ctx, repository.InsertNotificationParams{
		UserID:   userID,
		Category: msg.Category,
		Title:    msg.Title,
		Body:     msg.Body,
		Link:     msg.Link,
	})
}
