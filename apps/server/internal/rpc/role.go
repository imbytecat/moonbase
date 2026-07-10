package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	rolev1 "github.com/imbytecat/moonbase/server/internal/gen/role/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/role/v1/rolev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

type RoleService struct {
	repo   repository.Querier
	logger *slog.Logger
}

func NewRoleService(repo repository.Querier, logger *slog.Logger) *RoleService {
	return &RoleService{repo: repo, logger: logger}
}

var _ rolev1connect.RoleServiceHandler = (*RoleService)(nil)

func (s *RoleService) ListRoles(
	ctx context.Context,
	_ *connect.Request[rolev1.ListRolesRequest],
) (*connect.Response[rolev1.ListRolesResponse], error) {
	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list roles", err)
	}
	perms, err := s.rolePermissions(ctx)
	if err != nil {
		return nil, s.internal(ctx, "list role permissions", err)
	}
	out := make([]*rolev1.Role, len(roles))
	for i, r := range roles {
		out[i] = toProtoRole(r, perms[r.ID])
	}
	return connect.NewResponse(&rolev1.ListRolesResponse{Roles: out}), nil
}

func (s *RoleService) ListPermissions(
	_ context.Context,
	_ *connect.Request[rolev1.ListPermissionsRequest],
) (*connect.Response[rolev1.ListPermissionsResponse], error) {
	out := make([]*rolev1.PermissionInfo, len(auth.Catalog))
	for i, p := range auth.Catalog {
		out[i] = &rolev1.PermissionInfo{
			Permission:  permissionEnum(p.Key),
			Description: p.Description,
		}
	}
	return connect.NewResponse(&rolev1.ListPermissionsResponse{Permissions: out}), nil
}

func (s *RoleService) CreateRole(
	ctx context.Context,
	req *connect.Request[rolev1.CreateRoleRequest],
) (*connect.Response[rolev1.CreateRoleResponse], error) {
	keys := permissionKeys(req.Msg.GetPermissions())
	if err := validatePermissions(keys); err != nil {
		return nil, err
	}
	role, err := s.repo.CreateRole(ctx, repository.CreateRoleParams{
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
		IsSystem:    false,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(
				connect.CodeAlreadyExists,
				errors.New("role name already exists"),
			)
		}
		return nil, s.internal(ctx, "create role", err)
	}
	if len(keys) > 0 {
		if err := s.repo.AddRolePermissions(ctx, repository.AddRolePermissionsParams{
			RoleID:      role.ID,
			Permissions: keys,
		}); err != nil {
			return nil, s.internal(ctx, "grant permissions", err)
		}
	}
	return connect.NewResponse(&rolev1.CreateRoleResponse{
		Role: toProtoRole(role, keys),
	}), nil
}

func (s *RoleService) UpdateRole(
	ctx context.Context,
	req *connect.Request[rolev1.UpdateRoleRequest],
) (*connect.Response[rolev1.UpdateRoleResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	keys := permissionKeys(req.Msg.GetPermissions())
	if err := validatePermissions(keys); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
	}
	if err != nil {
		return nil, s.internal(ctx, "get role", err)
	}

	params := repository.UpdateRoleParams{
		ID:          id,
		Description: pgtype.Text{String: req.Msg.GetDescription(), Valid: true},
	}
	// System roles keep their name (admin/user are referenced by seed logic
	// and the register flow); their permission sets stay editable — EXCEPT the
	// admin role, whose wildcard must be immutable: the permission checkboxes
	// can't express "*", so a well-meaning edit would silently strip it and
	// could lock everyone out of role management.
	if !existing.IsSystem {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}
	role, err := s.repo.UpdateRole(ctx, params)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(
				connect.CodeAlreadyExists,
				errors.New("role name already exists"),
			)
		}
		return nil, s.internal(ctx, "update role", err)
	}

	perms, adminLocked := keys, isAdminRole(existing)
	if adminLocked {
		perms = []string{auth.WildcardPermission}
	} else {
		if err := s.repo.DeleteRolePermissions(ctx, id); err != nil {
			return nil, s.internal(ctx, "clear permissions", err)
		}
		if len(perms) > 0 {
			if err := s.repo.AddRolePermissions(ctx, repository.AddRolePermissionsParams{
				RoleID:      id,
				Permissions: perms,
			}); err != nil {
				return nil, s.internal(ctx, "grant permissions", err)
			}
		}
	}
	return connect.NewResponse(&rolev1.UpdateRoleResponse{
		Role: toProtoRole(role, perms),
	}), nil
}

func isAdminRole(r repository.Role) bool {
	return r.IsSystem && r.Name == "admin"
}

func (s *RoleService) DeleteRole(
	ctx context.Context,
	req *connect.Request[rolev1.DeleteRoleRequest],
) (*connect.Response[rolev1.DeleteRoleResponse], error) {
	id, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	role, err := s.repo.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
	}
	if err != nil {
		return nil, s.internal(ctx, "get role", err)
	}
	if role.IsSystem {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("system roles cannot be deleted"),
		)
	}
	count, err := s.repo.CountUsersWithRole(ctx, id)
	if err != nil {
		return nil, s.internal(ctx, "count role users", err)
	}
	if count > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("role is assigned to %d user(s); unassign it first", count))
	}
	if err := s.repo.DeleteRole(ctx, id); err != nil {
		return nil, s.internal(ctx, "delete role", err)
	}
	return connect.NewResponse(&rolev1.DeleteRoleResponse{}), nil
}

func (s *RoleService) rolePermissions(ctx context.Context) (map[uuid.UUID][]string, error) {
	rows, err := s.repo.ListRolePermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]string)
	for _, r := range rows {
		out[r.RoleID] = append(out[r.RoleID], r.Permission)
	}
	return out, nil
}

func toProtoRole(r repository.Role, perms []string) *rolev1.Role {
	return &rolev1.Role{
		Id:          r.ID.String(),
		Name:        r.Name,
		Description: r.Description,
		IsSystem:    r.IsSystem,
		Permissions: permissionEnums(perms),
		CreatedAt:   timestamppb.New(r.CreatedAt),
		UpdatedAt:   timestamppb.New(r.UpdatedAt),
	}
}

func validatePermissions(perms []string) error {
	for _, p := range perms {
		if !auth.IsKnownPermission(p) {
			return connect.NewError(
				connect.CodeInvalidArgument,
				fmt.Errorf("unknown permission %q", p),
			)
		}
	}
	return nil
}

func (s *RoleService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
