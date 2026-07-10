package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	notificationv1 "github.com/imbytecat/moonbase/server/internal/gen/notification/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/notification/v1/notificationv1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const defaultNotificationPageSize = 20

// NotificationService is the per-user inbox surface. Every RPC scopes to the
// caller's own identity (authz binds them all to any authenticated session),
// so no request can read or mutate another user's inbox.
type NotificationService struct {
	repo   repository.Querier
	logger *slog.Logger
}

func NewNotificationService(repo repository.Querier, logger *slog.Logger) *NotificationService {
	return &NotificationService{repo: repo, logger: logger}
}

var _ notificationv1connect.NotificationServiceHandler = (*NotificationService)(nil)

func (s *NotificationService) ListNotifications(
	ctx context.Context,
	req *connect.Request[notificationv1.ListNotificationsRequest],
) (*connect.Response[notificationv1.ListNotificationsResponse], error) {
	userID := auth.IdentityFromContext(ctx).UserID
	pageSize := req.Msg.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultNotificationPageSize
	}
	unreadOnly := req.Msg.GetUnreadOnly()

	rows, err := s.repo.ListNotifications(ctx, repository.ListNotificationsParams{
		Limit:      pageSize,
		Offset:     req.Msg.GetPage() * pageSize,
		UserID:     userID,
		UnreadOnly: unreadOnly,
	})
	if err != nil {
		return nil, s.internal(ctx, "list notifications", err)
	}
	total, err := s.repo.CountNotifications(ctx, repository.CountNotificationsParams{
		UserID:     userID,
		UnreadOnly: unreadOnly,
	})
	if err != nil {
		return nil, s.internal(ctx, "count notifications", err)
	}
	unread, err := s.repo.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return nil, s.internal(ctx, "count unread notifications", err)
	}

	out := make([]*notificationv1.Notification, len(rows))
	for i, row := range rows {
		out[i] = toProtoNotification(row)
	}
	return connect.NewResponse(&notificationv1.ListNotificationsResponse{
		Notifications: out,
		Total:         total,
		Unread:        unread,
	}), nil
}

func (s *NotificationService) GetUnreadCount(
	ctx context.Context,
	_ *connect.Request[notificationv1.GetUnreadCountRequest],
) (*connect.Response[notificationv1.GetUnreadCountResponse], error) {
	userID := auth.IdentityFromContext(ctx).UserID
	unread, err := s.repo.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return nil, s.internal(ctx, "count unread notifications", err)
	}
	return connect.NewResponse(&notificationv1.GetUnreadCountResponse{Unread: unread}), nil
}

func (s *NotificationService) MarkNotificationsRead(
	ctx context.Context,
	req *connect.Request[notificationv1.MarkNotificationsReadRequest],
) (*connect.Response[notificationv1.MarkNotificationsReadResponse], error) {
	userID := auth.IdentityFromContext(ctx).UserID
	ids, err := notificationIDs(req.Msg.GetIds())
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("invalid notification id"),
		)
	}
	if err := s.repo.MarkNotificationsRead(ctx, repository.MarkNotificationsReadParams{
		UserID: userID,
		Ids:    ids,
	}); err != nil {
		return nil, s.internal(ctx, "mark notifications read", err)
	}
	unread, err := s.unread(ctx, userID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&notificationv1.MarkNotificationsReadResponse{Unread: unread}), nil
}

func (s *NotificationService) MarkAllNotificationsRead(
	ctx context.Context,
	_ *connect.Request[notificationv1.MarkAllNotificationsReadRequest],
) (*connect.Response[notificationv1.MarkAllNotificationsReadResponse], error) {
	userID := auth.IdentityFromContext(ctx).UserID
	if err := s.repo.MarkAllNotificationsRead(ctx, userID); err != nil {
		return nil, s.internal(ctx, "mark all notifications read", err)
	}
	unread, err := s.unread(ctx, userID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(
		&notificationv1.MarkAllNotificationsReadResponse{Unread: unread},
	), nil
}

func (s *NotificationService) DeleteNotification(
	ctx context.Context,
	req *connect.Request[notificationv1.DeleteNotificationRequest],
) (*connect.Response[notificationv1.DeleteNotificationResponse], error) {
	userID := auth.IdentityFromContext(ctx).UserID
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("invalid notification id"),
		)
	}
	if err := s.repo.DeleteNotification(ctx, repository.DeleteNotificationParams{
		UserID: userID,
		ID:     id,
	}); err != nil {
		return nil, s.internal(ctx, "delete notification", err)
	}
	unread, err := s.unread(ctx, userID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&notificationv1.DeleteNotificationResponse{Unread: unread}), nil
}

func (s *NotificationService) unread(ctx context.Context, userID uuid.UUID) (int64, error) {
	unread, err := s.repo.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, s.internal(ctx, "count unread notifications", err)
	}
	return unread, nil
}

func notificationIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(raw))
	for i, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

func toProtoNotification(row repository.Notification) *notificationv1.Notification {
	out := &notificationv1.Notification{
		Id:        row.ID.String(),
		Category:  row.Category,
		Title:     row.Title,
		Body:      row.Body,
		Link:      row.Link,
		CreatedAt: timestamppb.New(row.CreatedAt),
	}
	if row.ReadAt.Valid {
		out.ReadAt = timestamppb.New(row.ReadAt.Time)
	}
	return out
}

func (s *NotificationService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
