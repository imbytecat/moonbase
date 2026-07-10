package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditv1 "github.com/imbytecat/moonbase/server/internal/gen/audit/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/audit/v1/auditv1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const defaultAuditPageSize = 20

// AuditService is the read-only surface over the append-only audit trail;
// writes happen exclusively in the audit interceptor.
type AuditService struct {
	repo   repository.Querier
	logger *slog.Logger
}

func NewAuditService(repo repository.Querier, logger *slog.Logger) *AuditService {
	return &AuditService{repo: repo, logger: logger}
}

var _ auditv1connect.AuditServiceHandler = (*AuditService)(nil)

func (s *AuditService) ListAuditLogs(
	ctx context.Context,
	req *connect.Request[auditv1.ListAuditLogsRequest],
) (*connect.Response[auditv1.ListAuditLogsResponse], error) {
	pageSize := req.Msg.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultAuditPageSize
	}

	var actorID pgtype.UUID
	if raw := req.Msg.GetActorId(); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("invalid actor id"),
			)
		}
		actorID = pgtype.UUID{Bytes: id, Valid: true}
	}
	var domain, action pgtype.Text
	if v := req.Msg.GetDomain(); v != "" {
		domain = pgtype.Text{String: v, Valid: true}
	}
	if v := req.Msg.GetAction(); v != "" {
		action = pgtype.Text{String: v, Valid: true}
	}
	var fromTime, toTime pgtype.Timestamptz
	if ts := req.Msg.GetFrom(); ts != nil {
		fromTime = pgtype.Timestamptz{Time: ts.AsTime(), Valid: true}
	}
	if ts := req.Msg.GetTo(); ts != nil {
		toTime = pgtype.Timestamptz{Time: ts.AsTime(), Valid: true}
	}

	rows, err := s.repo.ListAuditLogs(ctx, repository.ListAuditLogsParams{
		Limit:    pageSize,
		Offset:   req.Msg.GetPage() * pageSize,
		ActorID:  actorID,
		Domain:   domain,
		Action:   action,
		FromTime: fromTime,
		ToTime:   toTime,
	})
	if err != nil {
		return nil, s.internal(ctx, "list audit logs", err)
	}
	total, err := s.repo.CountAuditLogs(ctx, repository.CountAuditLogsParams{
		ActorID:  actorID,
		Domain:   domain,
		Action:   action,
		FromTime: fromTime,
		ToTime:   toTime,
	})
	if err != nil {
		return nil, s.internal(ctx, "count audit logs", err)
	}

	logs := make([]*auditv1.AuditLog, len(rows))
	for i, row := range rows {
		logs[i] = toProtoAuditLog(row)
	}
	return connect.NewResponse(&auditv1.ListAuditLogsResponse{Logs: logs, Total: total}), nil
}

func toProtoAuditLog(row repository.ListAuditLogsRow) *auditv1.AuditLog {
	out := &auditv1.AuditLog{
		Id:         row.ID.String(),
		ActorName:  row.ActorName,
		Action:     row.Action,
		Domain:     row.Domain,
		ResourceId: row.ResourceID,
		Result:     row.Result,
		Ip:         row.Ip,
		UserAgent:  row.UserAgent,
		CreatedAt:  timestamppb.New(row.CreatedAt),
	}
	if row.ActorID.Valid {
		out.ActorId = uuid.UUID(row.ActorID.Bytes).String()
	}
	return out
}

func (s *AuditService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
