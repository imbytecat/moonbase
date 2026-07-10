package pay

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

type SettlementEvent struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	Purpose           string
	BusinessReference string
	Type              string
}

type SettlementHandler interface {
	HandleSettlement(ctx context.Context, event SettlementEvent) error
}

type SettlementHandlerFunc func(context.Context, SettlementEvent) error

func (f SettlementHandlerFunc) HandleSettlement(ctx context.Context, event SettlementEvent) error {
	return f(ctx, event)
}

type SettlementDispatcher struct {
	repo     repository.Querier
	handlers map[string]SettlementHandler
	logger   *slog.Logger
}

func NewSettlementDispatcher(repo repository.Querier, handlers map[string]SettlementHandler, logger *slog.Logger) *SettlementDispatcher {
	return &SettlementDispatcher{repo: repo, handlers: handlers, logger: logger}
}

func (d *SettlementDispatcher) DispatchOne(ctx context.Context) (bool, error) {
	row, err := d.repo.ClaimSettlementEvent(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	event := SettlementEvent{
		ID: row.ID, OrderID: row.OrderID, Purpose: row.Purpose,
		BusinessReference: row.BusinessReference, Type: row.EventType,
	}
	handler := d.handlers[row.Purpose]
	if handler == nil {
		// Demo/template purposes may intentionally have no business handler.
		return true, d.repo.MarkSettlementEventDelivered(ctx, row.ID)
	}
	if err := handler.HandleSettlement(ctx, event); err != nil {
		d.logger.WarnContext(ctx, "payment settlement delivery failed", "event", row.ID, "error", err)
		delay := time.Second * time.Duration(1<<min(row.Attempts, 8))
		if retryErr := d.repo.RetrySettlementEvent(ctx, repository.RetrySettlementEventParams{
			ID: row.ID, NextAttemptAt: time.Now().Add(delay), LastError: err.Error(),
		}); retryErr != nil {
			return true, retryErr
		}
		return true, nil
	}
	return true, d.repo.MarkSettlementEventDelivered(ctx, row.ID)
}

func (d *SettlementDispatcher) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			for {
				worked, err := d.DispatchOne(ctx)
				if err != nil {
					d.logger.ErrorContext(ctx, "payment settlement dispatcher failed", "error", err)
					break
				}
				if !worked {
					break
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
