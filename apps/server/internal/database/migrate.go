package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/imbytecat/moonbase/server/db"
)

// Migrate applies the embedded goose migrations. A Postgres session-level
// advisory lock serializes concurrent replicas racing to migrate on startup.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create session locker: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer func() { _ = sqlDB.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.Migrations(),
		goose.WithSessionLocker(locker),
		goose.WithLogger(gooseLogger{logger.With("component", "goose")}))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

type gooseLogger struct {
	l *slog.Logger
}

func (g gooseLogger) Printf(format string, v ...any) {
	g.l.Info(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}

func (g gooseLogger) Fatalf(format string, v ...any) {
	g.l.Error(strings.TrimSuffix(fmt.Sprintf(format, v...), "\n"))
}
