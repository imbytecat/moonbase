package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

const (
	reclaimWorkflowName = "reclaim-unattached-files"
	// reclaimSchedule sweeps hourly (6-field cron, second precision). The 24h
	// grace period means a file need not be reclaimed the instant it qualifies,
	// so an hourly cadence is plenty.
	reclaimSchedule = "0 0 * * * *"
	// reclaimGracePeriod keeps a freshly presigned-but-unsaved file around long
	// enough for its business save to attach it before the sweep reclaims it.
	reclaimGracePeriod = 24 * time.Hour
)

// ReclaimStore is the ledger side of the sweep: find unattached files and drop
// their rows. repository.Querier satisfies it.
type ReclaimStore interface {
	ListUnattachedFiles(ctx context.Context, createdBefore time.Time) ([]repository.ListUnattachedFilesRow, error)
	DeleteFile(ctx context.Context, id uuid.UUID) error
}

// ObjectDeleter is the bucket side of the sweep. storage.ObjectStore satisfies
// it; Delete must be idempotent.
type ObjectDeleter interface {
	Delete(ctx context.Context, purpose, key string) error
}

// registerReclaim wires the unattached-file sweep as a scheduled DBOS workflow.
// One durable step runs the whole reclaim; because every effect is idempotent,
// a crash-resumed run simply re-lists and re-deletes, converging the
// "object gone, row still there" middle state.
func registerReclaim(dctx dbos.DBOSContext, store ReclaimStore, objects ObjectDeleter, logger *slog.Logger) {
	dbos.RegisterWorkflow(dctx,
		func(wctx dbos.DBOSContext, scheduledTime time.Time) (int, error) {
			return dbos.RunAsStep(wctx, func(ctx context.Context) (int, error) {
				return reclaimUnattached(ctx, store, objects, scheduledTime.Add(-reclaimGracePeriod), logger)
			}, dbos.WithStepName("reclaim-unattached"))
		},
		dbos.WithWorkflowName(reclaimWorkflowName),
		dbos.WithSchedule(reclaimSchedule),
	)
}

// reclaimUnattached deletes every file unattached since before cutoff, object
// first then ledger row (ADR-0003). The order matters: a crash between the two
// leaves an accounted-for row whose object is already gone, which the next
// sweep re-deletes (idempotent) and finishes — the reverse would strand an
// object with no account. Per-file failures are logged and skipped so one bad
// file never blocks the rest of the sweep.
func reclaimUnattached(ctx context.Context, store ReclaimStore, objects ObjectDeleter, cutoff time.Time, logger *slog.Logger) (int, error) {
	files, err := store.ListUnattachedFiles(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("list unattached files: %w", err)
	}
	reclaimed := 0
	for _, f := range files {
		if err := objects.Delete(ctx, f.Purpose, f.ObjectKey); err != nil {
			logger.ErrorContext(ctx, "reclaim: delete object failed, skipping",
				"file", f.ID, "purpose", f.Purpose, "error", err)
			continue
		}
		if err := store.DeleteFile(ctx, f.ID); err != nil {
			logger.ErrorContext(ctx, "reclaim: delete file row failed, skipping",
				"file", f.ID, "error", err)
			continue
		}
		reclaimed++
	}
	return reclaimed, nil
}
