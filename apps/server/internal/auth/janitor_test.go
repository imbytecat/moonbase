package auth

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// janitorFake is a Querier double whose expiry-cleanup methods report each
// invocation on a non-blocking channel, so a test can wait for the ticker to
// fire without ever blocking (and thus leaking) the janitor goroutine.
type janitorFake struct {
	repository.Querier
	ran chan struct{}
}

func (f *janitorFake) DeleteExpiredSessions(context.Context) error {
	select {
	case f.ran <- struct{}{}:
	default:
	}
	return nil
}

func (f *janitorFake) DeleteExpiredVerificationTokens(context.Context) error { return nil }

func (f *janitorFake) DeleteExpiredOauthSignupTickets(context.Context) error { return nil }

// TestStartSessionJanitorStopsOnCancel proves the background janitor is
// leak-free: goleak.VerifyNone fails the test if the goroutine is still alive
// after ctx cancellation. It first waits for one real tick so the assertion
// also confirms the loop was genuinely running (not exiting immediately).
func TestStartSessionJanitorStopsOnCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	fake := &janitorFake{ran: make(chan struct{}, 1)}
	StartSessionJanitor(ctx, fake, slog.New(slog.DiscardHandler), time.Millisecond)

	select {
	case <-fake.ran:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("janitor never ran")
	}

	cancel()
}
