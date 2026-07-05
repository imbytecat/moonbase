// Package database wires the pgx connection pool used by the repository layer.
package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	pgxslog "github.com/mcosta74/pgx-slog"
)

// NewPool connects with pgx internals (connect/acquire errors) logged to
// logger at warn level; logSQL additionally traces every statement with args
// and duration at debug level.
func NewPool(ctx context.Context, url string, logger *slog.Logger, logSQL bool) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	traceLevel := tracelog.LogLevelWarn
	if logSQL {
		traceLevel = tracelog.LogLevelDebug
	}
	cfg.ConnConfig.Tracer = &tracelog.TraceLog{
		Logger:   pgxslog.NewLogger(logger.With("component", "pgx")),
		LogLevel: traceLevel,
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
