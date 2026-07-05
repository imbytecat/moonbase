package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"google.golang.org/protobuf/types/known/timestamppb"

	workflowv1 "github.com/imbytecat/moonbase/server/internal/gen/workflow/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/workflow/v1/workflowv1connect"
	"github.com/imbytecat/moonbase/server/internal/workflow"
)

const defaultRunLimit = 50

// WorkflowService exposes durable-run observability and control. The engine
// is nil-able: builds without a workflow engine (unit tests) answer
// FailedPrecondition instead of crashing.
type WorkflowService struct {
	engine *workflow.Engine
	logger *slog.Logger
}

func NewWorkflowService(engine *workflow.Engine, logger *slog.Logger) *WorkflowService {
	return &WorkflowService{engine: engine, logger: logger}
}

var _ workflowv1connect.WorkflowServiceHandler = (*WorkflowService)(nil)

var errWorkflowsUnavailable = connect.NewError(connect.CodeFailedPrecondition,
	errors.New("workflow engine is not available"))

func (s *WorkflowService) ListWorkflowRuns(
	ctx context.Context,
	req *connect.Request[workflowv1.ListWorkflowRunsRequest],
) (*connect.Response[workflowv1.ListWorkflowRunsResponse], error) {
	if s.engine == nil {
		return nil, errWorkflowsUnavailable
	}
	limit := int(req.Msg.GetLimit())
	if limit == 0 {
		limit = defaultRunLimit
	}
	statuses, err := s.engine.List(limit)
	if err != nil {
		return nil, s.internal(ctx, "list workflows", err)
	}
	runs := make([]*workflowv1.WorkflowRun, len(statuses))
	for i, st := range statuses {
		runs[i] = toProtoRun(st)
	}
	return connect.NewResponse(&workflowv1.ListWorkflowRunsResponse{Runs: runs}), nil
}

func (s *WorkflowService) GetWorkflowRun(
	ctx context.Context,
	req *connect.Request[workflowv1.GetWorkflowRunRequest],
) (*connect.Response[workflowv1.GetWorkflowRunResponse], error) {
	if s.engine == nil {
		return nil, errWorkflowsUnavailable
	}
	id := req.Msg.GetId()
	runs, err := s.engine.ListByID(id)
	if err != nil {
		return nil, s.internal(ctx, "get workflow", err)
	}
	if len(runs) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("workflow run not found"))
	}
	steps, err := s.engine.Steps(id)
	if err != nil {
		return nil, s.internal(ctx, "get workflow steps", err)
	}
	out := &workflowv1.GetWorkflowRunResponse{
		Run:   toProtoRun(runs[0]),
		Steps: make([]*workflowv1.WorkflowStep, len(steps)),
	}
	for i, st := range steps {
		out.Steps[i] = toProtoStep(st)
	}
	return connect.NewResponse(out), nil
}

func (s *WorkflowService) CancelWorkflowRun(
	ctx context.Context,
	req *connect.Request[workflowv1.CancelWorkflowRunRequest],
) (*connect.Response[workflowv1.CancelWorkflowRunResponse], error) {
	if s.engine == nil {
		return nil, errWorkflowsUnavailable
	}
	if err := s.engine.Cancel(req.Msg.GetId()); err != nil {
		return nil, s.internal(ctx, "cancel workflow", err)
	}
	return connect.NewResponse(&workflowv1.CancelWorkflowRunResponse{}), nil
}

func (s *WorkflowService) ResumeWorkflowRun(
	ctx context.Context,
	req *connect.Request[workflowv1.ResumeWorkflowRunRequest],
) (*connect.Response[workflowv1.ResumeWorkflowRunResponse], error) {
	if s.engine == nil {
		return nil, errWorkflowsUnavailable
	}
	if err := s.engine.Resume(req.Msg.GetId()); err != nil {
		return nil, s.internal(ctx, "resume workflow", err)
	}
	return connect.NewResponse(&workflowv1.ResumeWorkflowRunResponse{}), nil
}

func (s *WorkflowService) TriggerDemoWorkflow(
	ctx context.Context,
	req *connect.Request[workflowv1.TriggerDemoWorkflowRequest],
) (*connect.Response[workflowv1.TriggerDemoWorkflowResponse], error) {
	if s.engine == nil {
		return nil, errWorkflowsUnavailable
	}
	id, err := s.engine.TriggerDemo(req.Msg.GetName())
	if err != nil {
		return nil, s.internal(ctx, "trigger demo workflow", err)
	}
	return connect.NewResponse(&workflowv1.TriggerDemoWorkflowResponse{Id: id}), nil
}

func toProtoRun(st dbos.WorkflowStatus) *workflowv1.WorkflowRun {
	out := &workflowv1.WorkflowRun{
		Id:               st.ID,
		Name:             st.Name,
		Status:           string(st.Status),
		Input:            compactJSON(st.Input),
		Output:           compactJSON(st.Output),
		Attempts:         int32(min(st.Attempts, 1<<30)), //nolint:gosec // display only
		ParentWorkflowId: st.ParentWorkflowID,
		CreatedAt:        timestamppb.New(st.CreatedAt),
		UpdatedAt:        timestamppb.New(st.UpdatedAt),
	}
	if st.Error != nil {
		out.Error = st.Error.Error()
	}
	if !st.CompletedAt.IsZero() {
		out.CompletedAt = timestamppb.New(st.CompletedAt)
	}
	return out
}

func toProtoStep(st dbos.StepInfo) *workflowv1.WorkflowStep {
	out := &workflowv1.WorkflowStep{
		StepId:          int32(min(st.StepID, 1<<30)), //nolint:gosec // display only
		StepName:        st.StepName,
		Output:          compactJSON(st.Output),
		ChildWorkflowId: st.ChildWorkflowID,
	}
	if st.Error != nil {
		out.Error = st.Error.Error()
	}
	if !st.StartedAt.IsZero() {
		out.StartedAt = timestamppb.New(st.StartedAt)
	}
	if !st.CompletedAt.IsZero() {
		out.CompletedAt = timestamppb.New(st.CompletedAt)
	}
	return out
}

// compactJSON renders workflow inputs/outputs for display. DBOS hands values
// back as JSON-encoded strings; unwrap that layer so the UI shows the actual
// value, and degrade gracefully for anything non-serializable.
func compactJSON(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err == nil {
			v = inner
		} else {
			return truncate(s)
		}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(raw)
	if s == "null" {
		return ""
	}
	return truncate(s)
}

func truncate(s string) string {
	const maxLen = 2048
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func (s *WorkflowService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
