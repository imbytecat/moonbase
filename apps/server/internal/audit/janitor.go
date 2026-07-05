package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// StartRetentionJanitor deletes audit rows older than retention on the given
// interval; retention <= 0 keeps everything forever. Deletion here is the
// ONLY way audit rows go away — the API surface is read-only by design.
func StartRetentionJanitor(ctx context.Context, repo repository.Querier, logger *slog.Logger, interval, retention time.Duration) {
	if retention <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := repo.DeleteAuditLogsBefore(ctx, time.Now().Add(-retention)); err != nil && ctx.Err() == nil {
					logger.ErrorContext(ctx, "audit log cleanup failed", "error", err)
				}
			}
		}
	}()
}
