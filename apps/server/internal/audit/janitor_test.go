package audit

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// retentionFake is a Querier double whose delete method reports each call on a
// non-blocking channel, so the test can observe a tick without ever blocking
// (and thus leaking) the janitor goroutine.
type retentionFake struct {
	repository.Querier
	ran chan struct{}
}

func (f *retentionFake) DeleteAuditLogsBefore(context.Context, time.Time) error {
	select {
	case f.ran <- struct{}{}:
	default:
	}
	return nil
}

// TestStartRetentionJanitorStopsOnCancel proves the retention janitor exits on
// ctx cancellation (goleak) after genuinely running at least one sweep.
func TestStartRetentionJanitorStopsOnCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	fake := &retentionFake{ran: make(chan struct{}, 1)}
	StartRetentionJanitor(ctx, fake, slog.New(slog.DiscardHandler), time.Millisecond, 24*time.Hour)

	select {
	case <-fake.ran:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("janitor never ran")
	}

	cancel()
}

// TestStartRetentionJanitorDisabledSpawnsNothing pins the retention<=0 branch:
// "keep forever" must not start a goroutine at all.
func TestStartRetentionJanitorDisabledSpawnsNothing(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartRetentionJanitor(ctx, &retentionFake{}, slog.New(slog.DiscardHandler), time.Millisecond, 0)
}
