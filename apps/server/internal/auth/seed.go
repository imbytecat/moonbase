package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// usernamePattern mirrors the proto rule on RegisterRequest.username: seed
// writes to the DB directly (no protovalidate interceptor), and a username of
// the wrong shape (say, starting with a digit) would route to phone login and
// never match — so a bad MOONBASE_AUTH_ADMIN_USERNAME must fail startup.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`)

// Seed idempotently creates the system roles and, when the users table is
// empty, the initial admin account. Runs at startup right after migrations —
// same philosophy: a fresh database becomes usable with zero manual steps.
// The admin has only the username identifier: a fabricated email couldn't
// receive password-reset mail anyway — bind a real one later via the profile
// page (code-verified).
func Seed(ctx context.Context, repo repository.Querier, logger *slog.Logger, adminUsername, adminPassword string) error {
	adminRole, err := ensureRole(ctx, repo, "admin", "Full access to everything", []string{WildcardPermission})
	if err != nil {
		return err
	}
	if _, err := ensureRole(ctx, repo, "user", "Default role for regular users", []string{"report.read"}); err != nil {
		return err
	}

	count, err := repo.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	if !usernamePattern.MatchString(adminUsername) {
		return fmt.Errorf("invalid admin username %q: must start with a letter and use only letters, digits, '.', '_' or '-' (3-32 chars)", adminUsername)
	}

	hash, err := HashPassword(adminPassword)
	if err != nil {
		return err
	}
	admin, err := repo.CreateUser(ctx, repository.CreateUserParams{
		Username:     adminUsername,
		Name:         "Admin",
		PasswordHash: hash,
	})
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	if err := repo.AddUserRoles(ctx, repository.AddUserRolesParams{
		UserID:  admin.ID,
		RoleIds: []uuid.UUID{adminRole},
	}); err != nil {
		return fmt.Errorf("assign admin role: %w", err)
	}
	logger.Info("seeded initial admin user", "username", adminUsername)
	return nil
}

func ensureRole(ctx context.Context, repo repository.Querier, name, description string, perms []string) (uuid.UUID, error) {
	role, err := repo.GetRoleByName(ctx, name)
	if err == nil {
		return role.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("get role %s: %w", name, err)
	}
	role, err = repo.CreateRole(ctx, repository.CreateRoleParams{
		Name:        name,
		Description: description,
		IsSystem:    true,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create role %s: %w", name, err)
	}
	if err := repo.AddRolePermissions(ctx, repository.AddRolePermissionsParams{
		RoleID:      role.ID,
		Permissions: perms,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("grant permissions to %s: %w", name, err)
	}
	return role.ID, nil
}
