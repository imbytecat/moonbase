package rpc

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	userv1 "github.com/imbytecat/moonbase/server/internal/gen/user/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/user/v1/userv1connect"
	"github.com/imbytecat/moonbase/server/internal/i18n"
	"github.com/imbytecat/moonbase/server/internal/notification"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

type UserService struct {
	repo     repository.Querier
	objects  storage.ObjectStore
	notifier notification.Publisher
	logger   *slog.Logger
}

func NewUserService(repo repository.Querier, objects storage.ObjectStore, notifier notification.Publisher, logger *slog.Logger) *UserService {
	return &UserService{repo: repo, objects: objects, notifier: notifier, logger: logger}
}

var _ userv1connect.UserServiceHandler = (*UserService)(nil)

func (s *UserService) ListUsers(
	ctx context.Context,
	_ *connect.Request[userv1.ListUsersRequest],
) (*connect.Response[userv1.ListUsersResponse], error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list users", err)
	}
	assignments, err := s.userRoles(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list user roles", err)
	}
	out := make([]*userv1.User, len(users))
	for i, u := range users {
		out[i] = s.toProto(ctx, u, assignments[u.ID])
	}
	return connect.NewResponse(&userv1.ListUsersResponse{Users: out}), nil
}

func (s *UserService) CreateUser(
	ctx context.Context,
	req *connect.Request[userv1.CreateUserRequest],
) (*connect.Response[userv1.CreateUserResponse], error) {
	if req.Msg.GetUsername() == "" && req.Msg.GetEmail() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("username or email is required"))
	}
	hash, err := auth.HashPassword(req.Msg.GetPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	user, err := s.repo.CreateUser(ctx, repository.CreateUserParams{
		Username:     req.Msg.GetUsername(),
		Email:        req.Msg.GetEmail(),
		Name:         req.Msg.GetName(),
		PasswordHash: hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("username or email is already registered"))
		}
		return nil, s.internal(ctx, "create user", err)
	}
	roleIDs, err := parseUUIDs(req.Msg.GetRoleIds())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid role id"))
	}
	if len(roleIDs) > 0 {
		if err := s.repo.AddUserRoles(ctx, repository.AddUserRolesParams{
			UserID:  user.ID,
			RoleIds: roleIDs,
		}); err != nil {
			return nil, s.internal(ctx, "assign roles", err)
		}
	}
	assignments, err := s.userRoles(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list user roles", err)
	}
	return connect.NewResponse(&userv1.CreateUserResponse{
		User: s.toProto(ctx, user, assignments[user.ID]),
	}), nil
}

func (s *UserService) UpdateUser(
	ctx context.Context,
	req *connect.Request[userv1.UpdateUserRequest],
) (*connect.Response[userv1.UpdateUserResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	user, err := s.repo.UpdateUser(ctx, repository.UpdateUserParams{
		ID:       id,
		Email:    textArg(req.Msg.Email),
		Name:     textArg(req.Msg.Name),
		IsActive: boolArg(req.Msg.IsActive),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email is already registered"))
		}
		return nil, s.internal(ctx, "update user", err)
	}

	if req.Msg.Roles != nil {
		roleIDs, err := parseUUIDs(req.Msg.GetRoles().GetRoleIds())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid role id"))
		}
		before, err := s.roleIDsForUser(ctx, id)
		if err != nil {
			return nil, s.internal(ctx, "list user roles", err)
		}
		if err := s.repo.DeleteUserRoles(ctx, id); err != nil {
			return nil, s.internal(ctx, "clear roles", err)
		}
		if len(roleIDs) > 0 {
			if err := s.repo.AddUserRoles(ctx, repository.AddUserRolesParams{
				UserID:  id,
				RoleIds: roleIDs,
			}); err != nil {
				return nil, s.internal(ctx, "assign roles", err)
			}
		}
		if !sameIDSet(before, roleIDs) {
			s.notifyRoleChange(ctx, id)
		}
	}
	// Deactivating a user revokes their sessions immediately.
	if req.Msg.IsActive != nil && !req.Msg.GetIsActive() {
		if err := s.repo.DeleteUserSessions(ctx, id); err != nil {
			return nil, s.internal(ctx, "revoke sessions", err)
		}
	}

	assignments, err := s.userRoles(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list user roles", err)
	}
	return connect.NewResponse(&userv1.UpdateUserResponse{
		User: s.toProto(ctx, user, assignments[user.ID]),
	}), nil
}

func (s *UserService) DeleteUser(
	ctx context.Context,
	req *connect.Request[userv1.DeleteUserRequest],
) (*connect.Response[userv1.DeleteUserResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	if caller := auth.IdentityFromContext(ctx); caller != nil && caller.UserID == id {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot delete your own account"))
	}
	if err := s.repo.DeleteUser(ctx, id); err != nil {
		return nil, s.internal(ctx, "delete user", err)
	}
	return connect.NewResponse(&userv1.DeleteUserResponse{}), nil
}

// ResetUserPassword is the admin fallback for accounts that cannot self-reset
// (username-only accounts have no channel for the emailed reset link). Every
// session of the target user is revoked, same as the self-service reset.
func (s *UserService) ResetUserPassword(
	ctx context.Context,
	req *connect.Request[userv1.ResetUserPasswordRequest],
) (*connect.Response[userv1.ResetUserPasswordResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	if _, err := s.repo.GetUser(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	} else if err != nil {
		return nil, s.internal(ctx, "get user", err)
	}
	hash, err := auth.HashPassword(req.Msg.GetNewPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	if err := s.repo.UpdateUserPassword(ctx, repository.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: hash,
	}); err != nil {
		return nil, s.internal(ctx, "update password", err)
	}
	if err := s.repo.DeleteUserSessions(ctx, id); err != nil {
		return nil, s.internal(ctx, "revoke sessions", err)
	}
	return connect.NewResponse(&userv1.ResetUserPasswordResponse{}), nil
}

type roleRef struct {
	id   uuid.UUID
	name string
}

func (s *UserService) userRoles(ctx context.Context) (map[uuid.UUID][]roleRef, error) {
	rows, err := s.repo.ListUserRolesWithIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]roleRef)
	for _, r := range rows {
		out[r.UserID] = append(out[r.UserID], roleRef{id: r.RoleID, name: r.Name})
	}
	return out, nil
}

func (s *UserService) roleIDsForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.repo.ListUserRolesWithIDs(ctx)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, r := range rows {
		if r.UserID == userID {
			ids = append(ids, r.RoleID)
		}
	}
	return ids, nil
}

func sameIDSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[uuid.UUID]struct{}, len(a))
	for _, x := range a {
		seen[x] = struct{}{}
	}
	for _, x := range b {
		if _, ok := seen[x]; !ok {
			return false
		}
	}
	return true
}

// notifyRoleChange best-effort tells the affected user their roles changed; a
// failure must never fail the admin's update, so it only logs.
func (s *UserService) notifyRoleChange(ctx context.Context, userID uuid.UUID) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.Publish(ctx, userID, notification.Message{
		Category: notification.CategoryAccount,
		Link:     "/profile",
		TitleKey: i18n.NotifRoleChangedTitle,
		BodyKey:  i18n.NotifRoleChangedBody,
	}); err != nil {
		s.logger.WarnContext(ctx, "notify role change failed", "error", err)
	}
}

func (s *UserService) toProto(ctx context.Context, u repository.User, roles []roleRef) *userv1.User {
	names := make([]string, len(roles))
	ids := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.name
		ids[i] = r.id.String()
	}
	out := &userv1.User{
		Id:        u.ID.String(),
		Username:  u.Username,
		Email:     u.Email,
		Name:      u.Name,
		IsActive:  u.IsActive,
		Roles:     names,
		RoleIds:   ids,
		CreatedAt: timestamppb.New(u.CreatedAt),
		UpdatedAt: timestamppb.New(u.UpdatedAt),
	}
	if u.AvatarKey != "" {
		if url, err := s.objects.ResolveURL(ctx, storage.PurposeAvatars, u.AvatarKey, avatarURLTTL); err == nil {
			out.AvatarUrl = url
		}
	}
	return out
}

func (s *UserService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func parseUUIDs(in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, len(in))
	for i, s := range in {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
