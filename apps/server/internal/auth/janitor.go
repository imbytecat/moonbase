package auth

import (
	"context"
	"log/slog"
	"time"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// StartSessionJanitor periodically deletes expired session and verification
// token rows until ctx is cancelled. Expired rows are already unusable (every
// lookup filters on expiry); this only keeps the tables from growing forever.
func StartSessionJanitor(ctx context.Context, repo repository.Querier, logger *slog.Logger, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := repo.DeleteExpiredSessions(ctx); err != nil && ctx.Err() == nil {
					logger.ErrorContext(ctx, "expired session cleanup failed", "error", err)
				}
				if err := repo.DeleteExpiredVerificationTokens(ctx); err != nil && ctx.Err() == nil {
					logger.ErrorContext(ctx, "expired verification token cleanup failed", "error", err)
				}
				if err := repo.DeleteExpiredOauthSignupTickets(ctx); err != nil && ctx.Err() == nil {
					logger.ErrorContext(ctx, "expired oauth signup ticket cleanup failed", "error", err)
				}
			}
		}
	}()
}
