// Package workflow wraps DBOS Transact: durable, Postgres-backed workflow
// orchestration inside the app binary — no external orchestrator. Workflows
// checkpoint every step into the "dbos" schema of the SAME Postgres the app
// already uses; on crash/restart they resume from the last completed step.
//
// The catalog of runnable workflows is code (like permissions and storage
// purposes): register functions here, expose triggers via workflow.v1 RPCs.
package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// DBOS registers workflows under the function's fully-qualified name; the
// demo's ends with this suffix (matched by IsDemoWorkflow).
const demoWorkflowSuffix = ".demoWorkflow"

type Engine struct {
	ctx    dbos.DBOSContext
	logger *slog.Logger
}

// New initializes DBOS against the given database URL and launches the
// executor. Call Shutdown on process exit. store and objects back the scheduled
// unattached-file sweep.
func New(ctx context.Context, databaseURL, appName string, store ReclaimStore, objects ObjectDeleter, logger *slog.Logger) (*Engine, error) {
	dctx, err := dbos.NewDBOSContext(ctx, dbos.Config{
		AppName:     appName,
		DatabaseURL: databaseURL,
		Logger:      logger,
	})
	if err != nil {
		return nil, fmt.Errorf("init workflow engine: %w", err)
	}
	dbos.RegisterWorkflow(dctx, demoWorkflow)
	registerReclaim(dctx, store, objects, logger)
	if err := dbos.Launch(dctx); err != nil {
		return nil, fmt.Errorf("launch workflow engine: %w", err)
	}
	return &Engine{ctx: dctx, logger: logger}, nil
}

func (e *Engine) Shutdown(timeout time.Duration) {
	dbos.Shutdown(e.ctx, timeout)
}

func (e *Engine) List(limit int) ([]dbos.WorkflowStatus, error) {
	return dbos.ListWorkflows(e.ctx,
		dbos.WithLimit(limit),
		dbos.WithSortDesc(),
		dbos.WithLoadInput(true),
		dbos.WithLoadOutput(true),
	)
}

func (e *Engine) ListByID(workflowID string) ([]dbos.WorkflowStatus, error) {
	return dbos.ListWorkflows(e.ctx,
		dbos.WithWorkflowIDs([]string{workflowID}),
		dbos.WithLoadInput(true),
		dbos.WithLoadOutput(true),
	)
}

func (e *Engine) Steps(workflowID string) ([]dbos.StepInfo, error) {
	return dbos.GetWorkflowSteps(e.ctx, workflowID)
}

func (e *Engine) Cancel(workflowID string) error {
	return dbos.CancelWorkflow(e.ctx, workflowID)
}

func (e *Engine) Resume(workflowID string) error {
	_, err := dbos.ResumeWorkflow[string](e.ctx, workflowID)
	return err
}

// TriggerDemo starts the demo workflow asynchronously and returns its id.
func (e *Engine) TriggerDemo(name string) (string, error) {
	handle, err := dbos.RunWorkflow(e.ctx, demoWorkflow, name)
	if err != nil {
		return "", err
	}
	return handle.GetWorkflowID(), nil
}

// IsDemoWorkflow reports whether a status row is the built-in demo (the UI
// labels it distinctly).
func IsDemoWorkflow(name string) bool {
	return strings.HasSuffix(name, demoWorkflowSuffix)
}

// demoWorkflow demonstrates durable execution: three checkpointed steps with
// a durable sleep in between. Kill the server mid-run and restart — the run
// resumes after the last completed step instead of starting over.
func demoWorkflow(ctx dbos.DBOSContext, name string) (string, error) {
	prepared, err := dbos.RunAsStep(ctx, func(context.Context) (string, error) {
		return fmt.Sprintf("prepared:%s", name), nil
	}, dbos.WithStepName("prepare"))
	if err != nil {
		return "", err
	}

	if _, err := dbos.Sleep(ctx, 5*time.Second); err != nil {
		return "", err
	}

	processed, err := dbos.RunAsStep(ctx, func(context.Context) (string, error) {
		return strings.ToUpper(prepared), nil
	}, dbos.WithStepName("process"))
	if err != nil {
		return "", err
	}

	return dbos.RunAsStep(ctx, func(context.Context) (string, error) {
		return fmt.Sprintf("done:%s:%d", processed, time.Now().Unix()), nil
	}, dbos.WithStepName("finalize"))
}
