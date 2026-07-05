package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	reportv1 "github.com/imbytecat/moonbase/server/internal/gen/report/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/report/v1/reportv1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/workflow"
)

// ReportService aggregates real system data (users, sessions, identities,
// roles, workflow runs) into dashboard shapes. All counting happens in SQL;
// this layer only maps rows to proto.
type ReportService struct {
	repo   repository.Querier
	engine *workflow.Engine
	logger *slog.Logger
}

// NewReportService wires the report aggregates. engine may be nil (tests
// without a workflow executor) — the workflow breakdown is then empty rather
// than an error, since the rest of the report is still meaningful.
func NewReportService(repo repository.Querier, engine *workflow.Engine, logger *slog.Logger) *ReportService {
	return &ReportService{repo: repo, engine: engine, logger: logger}
}

var _ reportv1connect.ReportServiceHandler = (*ReportService)(nil)

// workflowRunSample bounds how many recent runs feed the status breakdown;
// DBOS has no aggregate API, so we classify a bounded recent window.
const workflowRunSample = 500

func (s *ReportService) GetDashboardReport(
	ctx context.Context,
	req *connect.Request[reportv1.GetDashboardReportRequest],
) (*connect.Response[reportv1.GetDashboardReportResponse], error) {
	days := req.Msg.GetDays()

	totalUsers, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, s.internal(ctx, "count users", err)
	}
	activeUsers, err := s.repo.CountActiveUsers(ctx)
	if err != nil {
		return nil, s.internal(ctx, "count active users", err)
	}
	newUsers, err := s.repo.CountUsersCreatedSince(ctx, days)
	if err != nil {
		return nil, s.internal(ctx, "count new users", err)
	}
	activeSessions, err := s.repo.CountActiveSessions(ctx)
	if err != nil {
		return nil, s.internal(ctx, "count active sessions", err)
	}

	signups, err := s.repo.UserSignupsByDay(ctx, days)
	if err != nil {
		return nil, s.internal(ctx, "user signups by day", err)
	}
	logins, err := s.repo.LoginsByDay(ctx, days)
	if err != nil {
		return nil, s.internal(ctx, "logins by day", err)
	}
	identities, err := s.repo.IdentitiesByProvider(ctx)
	if err != nil {
		return nil, s.internal(ctx, "identities by provider", err)
	}
	roles, err := s.repo.UsersByRole(ctx)
	if err != nil {
		return nil, s.internal(ctx, "users by role", err)
	}

	resp := &reportv1.GetDashboardReportResponse{
		TotalUsers:           totalUsers,
		ActiveUsers:          activeUsers,
		NewUsers:             newUsers,
		ActiveSessions:       activeSessions,
		UserSignups:          make([]*reportv1.MetricPoint, len(signups)),
		Logins:               make([]*reportv1.MetricPoint, len(logins)),
		WorkflowRunsByStatus: s.workflowRunsByStatus(ctx),
		IdentitiesByProvider: make([]*reportv1.NamedCount, len(identities)),
		UsersByRole:          make([]*reportv1.NamedCount, len(roles)),
	}
	for i, row := range signups {
		resp.UserSignups[i] = &reportv1.MetricPoint{
			Date:  row.Day.Time.Format("2006-01-02"),
			Count: row.Count,
		}
	}
	for i, row := range logins {
		resp.Logins[i] = &reportv1.MetricPoint{
			Date:  row.Day.Time.Format("2006-01-02"),
			Count: row.Count,
		}
	}
	for i, row := range identities {
		resp.IdentitiesByProvider[i] = &reportv1.NamedCount{Label: row.Provider, Count: row.Count}
	}
	for i, row := range roles {
		resp.UsersByRole[i] = &reportv1.NamedCount{Label: row.Name, Count: row.Count}
	}
	return connect.NewResponse(resp), nil
}

// workflowRunsByStatus classifies recent DBOS runs by status. Best-effort: a
// nil engine or a listing error yields an empty breakdown — the dashboard
// still renders everything else.
func (s *ReportService) workflowRunsByStatus(ctx context.Context) []*reportv1.NamedCount {
	if s.engine == nil {
		return nil
	}
	runs, err := s.engine.List(workflowRunSample)
	if err != nil {
		s.logger.WarnContext(ctx, "workflow status breakdown unavailable", "error", err)
		return nil
	}
	counts := map[string]int64{}
	order := []string{}
	for _, run := range runs {
		status := string(run.Status)
		if _, seen := counts[status]; !seen {
			order = append(order, status)
		}
		counts[status]++
	}
	out := make([]*reportv1.NamedCount, len(order))
	for i, status := range order {
		out[i] = &reportv1.NamedCount{Label: status, Count: counts[status]}
	}
	return out
}

func (s *ReportService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
