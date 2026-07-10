package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/imbytecat/moonbase/server/internal/audit"
	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/buildinfo"
	"github.com/imbytecat/moonbase/server/internal/config"
	"github.com/imbytecat/moonbase/server/internal/database"
	"github.com/imbytecat/moonbase/server/internal/logging"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/server"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/tracing"
	"github.com/imbytecat/moonbase/server/internal/workflow"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// The unified logger: console + optional rotating file sink. slog.SetDefault
	// also redirects the stdlib log package, so any third-party code logging
	// via `log` or slog's default lands in the same sinks.
	logger, closeLogger, err := logging.New(cfg.Log)
	if err != nil {
		return err
	}
	defer func() { _ = closeLogger() }()
	logger = slog.New(tracing.NewSlogHandler(logger.Handler()))
	slog.SetDefault(logger)

	logger.Info("starting moonbase", "build", buildinfo.Get())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	traceShutdown, err := tracing.Setup(ctx, cfg.Otel)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = traceShutdown(shutdownCtx)
	}()

	pool, err := database.NewPool(ctx, cfg.Database.URL, logger, cfg.Log.SQL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, logger); err != nil {
		return err
	}

	if err := auth.Seed(ctx, repository.New(pool), logger, cfg.Auth.AdminUsername, cfg.Auth.AdminPassword); err != nil {
		return err
	}

	// Carry any pre-ledger site branding onto file references; idempotent, so
	// it is safe on every startup (ADR-0003 存量回填).
	if err := settings.BackfillSiteAssets(ctx, pool, logger); err != nil {
		return err
	}

	auth.StartSessionJanitor(ctx, repository.New(pool), logger, time.Hour)
	audit.StartRetentionJanitor(ctx, repository.New(pool), logger, time.Hour, cfg.Audit.Retention())
	pay.NewSettlementDispatcher(repository.New(pool), nil, logger).Start(ctx, time.Second)

	// Durable workflows: DBOS checkpoints into the "dbos" schema of the same
	// Postgres. The engine recovers interrupted runs on startup and runs the
	// scheduled unattached-file sweep against storage + the file ledger.
	reclaimRepo := repository.New(pool)
	reclaimObjects := storage.NewClient(settings.NewStore(reclaimRepo))
	engine, err := workflow.New(ctx, cfg.Database.URL, "moonbase", reclaimRepo, reclaimObjects, logger)
	if err != nil {
		return err
	}
	defer engine.Shutdown(10 * time.Second)

	srv := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           server.NewRouter(cfg, pool, engine, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Server.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
