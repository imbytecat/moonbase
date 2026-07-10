package pay

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

type settlementRepo struct {
	repository.Querier
	event      repository.PaymentSettlementEvent
	delivered  bool
	retryCount int
}

func (r *settlementRepo) ClaimSettlementEvent(
	context.Context,
) (repository.PaymentSettlementEvent, error) {
	if r.delivered {
		return repository.PaymentSettlementEvent{}, pgx.ErrNoRows
	}
	r.event.Attempts++
	return r.event, nil
}

func (r *settlementRepo) MarkSettlementEventDelivered(context.Context, uuid.UUID) error {
	r.delivered = true
	return nil
}

func (r *settlementRepo) RetrySettlementEvent(
	_ context.Context,
	arg repository.RetrySettlementEventParams,
) error {
	r.retryCount++
	r.event.LastError = arg.LastError
	return nil
}

func TestSettlementDispatcherRetriesThenDelivers(t *testing.T) {
	repo := &settlementRepo{event: repository.PaymentSettlementEvent{
		ID: uuid.New(), OrderID: uuid.New(), Purpose: PurposeCheckout,
		BusinessReference: "order-42", EventType: "paid", CreatedAt: time.Now(),
	}}
	calls := 0
	dispatcher := NewSettlementDispatcher(repo, map[string]SettlementHandler{
		PurposeCheckout: SettlementHandlerFunc(func(context.Context, SettlementEvent) error {
			calls++
			if calls == 1 {
				return errors.New("temporary failure")
			}
			return nil
		}),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if worked, err := dispatcher.DispatchOne(t.Context()); err != nil || !worked {
		t.Fatalf("first DispatchOne() = (%v, %v), want retried work", worked, err)
	}
	if repo.retryCount != 1 || repo.delivered {
		t.Fatalf("after failure retry=%d delivered=%v", repo.retryCount, repo.delivered)
	}
	if worked, err := dispatcher.DispatchOne(t.Context()); err != nil || !worked {
		t.Fatalf("second DispatchOne() = (%v, %v), want delivered work", worked, err)
	}
	if calls != 2 || !repo.delivered {
		t.Fatalf("calls=%d delivered=%v, want two attempts and delivered", calls, repo.delivered)
	}
}
