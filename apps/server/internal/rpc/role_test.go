package rpc

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	rolev1 "github.com/imbytecat/moonbase/server/internal/gen/role/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

type fakeRoleQuerier struct {
	repository.Querier
	getRole               func(ctx context.Context, id uuid.UUID) (repository.Role, error)
	updateRole            func(ctx context.Context, arg repository.UpdateRoleParams) (repository.Role, error)
	deleteRolePermissions func(ctx context.Context, roleID uuid.UUID) error
	addRolePermissions    func(ctx context.Context, arg repository.AddRolePermissionsParams) error
}

func (f *fakeRoleQuerier) GetRole(ctx context.Context, id uuid.UUID) (repository.Role, error) {
	return f.getRole(ctx, id)
}

func (f *fakeRoleQuerier) UpdateRole(ctx context.Context, arg repository.UpdateRoleParams) (repository.Role, error) {
	return f.updateRole(ctx, arg)
}

func (f *fakeRoleQuerier) DeleteRolePermissions(ctx context.Context, roleID uuid.UUID) error {
	return f.deleteRolePermissions(ctx, roleID)
}

func (f *fakeRoleQuerier) AddRolePermissions(ctx context.Context, arg repository.AddRolePermissionsParams) error {
	return f.addRolePermissions(ctx, arg)
}

// UpdateRole on the admin role must never touch its permission set: the
// checkbox UI can't express "*", so honoring the request would silently strip
// the wildcard and could lock everyone out of role management.
func TestUpdateRoleAdminWildcardImmutable(t *testing.T) {
	adminID := uuid.New()
	svc := NewRoleService(&fakeRoleQuerier{
		getRole: func(context.Context, uuid.UUID) (repository.Role, error) {
			return repository.Role{ID: adminID, Name: "admin", IsSystem: true}, nil
		},
		updateRole: func(_ context.Context, arg repository.UpdateRoleParams) (repository.Role, error) {
			if arg.Name.Valid {
				t.Fatal("system role name must not be updatable")
			}
			return repository.Role{ID: adminID, Name: "admin", IsSystem: true, Description: arg.Description.String}, nil
		},
		deleteRolePermissions: func(context.Context, uuid.UUID) error {
			t.Fatal("admin permissions must not be cleared")
			return nil
		},
		addRolePermissions: func(context.Context, repository.AddRolePermissionsParams) error {
			t.Fatal("admin permissions must not be rewritten")
			return nil
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	resp, err := svc.UpdateRole(t.Context(), connect.NewRequest(&rolev1.UpdateRoleRequest{
		Id:          adminID.String(),
		Name:        "admin",
		Description: "still the admin",
		Permissions: []authv1.Permission{authv1.Permission_PERMISSION_REPORT_READ},
	}))
	if err != nil {
		t.Fatal(err)
	}
	perms := resp.Msg.GetRole().GetPermissions()
	if len(perms) != 1 || perms[0] != authv1.Permission_PERMISSION_ALL {
		t.Fatalf("admin permissions = %v, want [*]", perms)
	}
}
