package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/imbytecat/moonbase/server/internal/auth"
	authv1 "github.com/imbytecat/moonbase/server/internal/gen/auth/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/verify"
)

// CompleteOauthSignup turns a verified external identity (the ticket) into a
// local account. The completion form enforces the SAME signup policy as
// Register — the resulting account always satisfies the has-identifier CHECK
// and can log in without the provider.
func (s *AuthService) CompleteOauthSignup(
	ctx context.Context,
	req *connect.Request[authv1.CompleteOauthSignupRequest],
) (*connect.Response[authv1.CompleteOauthSignupResponse], error) {
	authCfg, err := s.settings.Auth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load auth settings", err)
	}
	if !authCfg.RegistrationEnabled {
		return nil, connect.NewError(
			connect.CodePermissionDenied,
			errors.New("registration is disabled"),
		)
	}

	ticket, err := s.repo.ConsumeOauthSignupTicket(ctx, auth.HashSessionToken(req.Msg.GetTicket()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("invalid or expired ticket"),
		)
	}
	if err != nil {
		return nil, s.internal(ctx, "consume signup ticket", err)
	}

	values, err := signupIdentifierValues(authCfg, &authv1.RegisterRequest{
		Username:  req.Msg.GetUsername(),
		Email:     req.Msg.GetEmail(),
		EmailCode: req.Msg.GetEmailCode(),
		Phone:     req.Msg.GetPhone(),
		PhoneCode: req.Msg.GetPhoneCode(),
	})
	if err != nil {
		return nil, err
	}
	if values.phone != "" {
		e164, err := s.normalizedAllowedPhone(ctx, values.phone)
		if err != nil {
			return nil, err
		}
		values.phone = e164
		if _, err := s.verifier.ConsumeCode(
			ctx,
			verify.PurposePhoneRegister,
			e164,
			req.Msg.GetPhoneCode(),
		); err != nil {
			return nil, s.verifyConsumeError(ctx, "consume phone register code", err)
		}
	}
	if values.email != "" {
		normalized := strings.ToLower(values.email)
		if _, err := s.verifier.ConsumeCode(
			ctx,
			verify.PurposeEmailRegister,
			normalized,
			req.Msg.GetEmailCode(),
		); err != nil {
			return nil, s.verifyConsumeError(ctx, "consume email register code", err)
		}
		values.emailVerified = true
	}

	hash, err := auth.HashPassword(req.Msg.GetPassword())
	if err != nil {
		return nil, s.internal(ctx, "hash password", err)
	}
	user, err := s.repo.CreateUser(ctx, repository.CreateUserParams{
		Username:      values.username,
		Email:         values.email,
		Name:          req.Msg.GetName(),
		PasswordHash:  hash,
		Phone:         values.phone,
		EmailVerified: values.emailVerified,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(
				connect.CodeAlreadyExists,
				errors.New("this account is already registered"),
			)
		}
		return nil, s.internal(ctx, "create user", err)
	}
	if _, err := s.repo.CreateIdentity(ctx, repository.CreateIdentityParams{
		UserID:     user.ID,
		Provider:   ticket.Provider,
		ProviderID: ticket.ProviderID,
		Name:       ticket.Name,
		AvatarUrl:  ticket.AvatarUrl,
	}); err != nil {
		return nil, s.internal(ctx, "create identity", err)
	}

	if role, err := s.repo.GetRoleByName(ctx, "user"); err == nil {
		if err := s.repo.AddUserRoles(ctx, repository.AddUserRolesParams{
			UserID:  user.ID,
			RoleIds: []uuid.UUID{role.ID},
		}); err != nil {
			return nil, s.internal(ctx, "assign default role", err)
		}
	}

	token, identity, err := s.createSession(ctx, user.ID, deviceInfo(req.Header(), req.Peer().Addr))
	if err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&authv1.CompleteOauthSignupResponse{
		User:         s.currentUser(ctx, identity),
		SessionToken: token,
	})
	resp.Header().Add("Set-Cookie", s.sessionCookie(token, s.policy.TTL).String())
	return resp, nil
}

func (s *AuthService) ListMyIdentities(
	ctx context.Context,
	_ *connect.Request[authv1.ListMyIdentitiesRequest],
) (*connect.Response[authv1.ListMyIdentitiesResponse], error) {
	id := auth.IdentityFromContext(ctx)
	rows, err := s.repo.ListUserIdentities(ctx, id.UserID)
	if err != nil {
		return nil, s.internal(ctx, "list identities", err)
	}
	out := make([]*authv1.OauthIdentity, len(rows))
	for i, row := range rows {
		out[i] = &authv1.OauthIdentity{
			ProviderKey: row.Provider,
			Name:        row.Name,
			CreatedAt:   timestamppb.New(row.CreatedAt),
		}
	}
	return connect.NewResponse(&authv1.ListMyIdentitiesResponse{Identities: out}), nil
}

func (s *AuthService) UnbindOauthIdentity(
	ctx context.Context,
	req *connect.Request[authv1.UnbindOauthIdentityRequest],
) (*connect.Response[authv1.UnbindOauthIdentityResponse], error) {
	id := auth.IdentityFromContext(ctx)
	if err := s.requirePassword(ctx, id.UserID, req.Msg.GetCurrentPassword()); err != nil {
		return nil, err
	}
	rows, err := s.repo.DeleteUserIdentity(ctx, repository.DeleteUserIdentityParams{
		UserID:   id.UserID,
		Provider: req.Msg.GetProviderKey(),
	})
	if err != nil {
		return nil, s.internal(ctx, "unbind identity", err)
	}
	if rows == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("identity not found"))
	}
	return connect.NewResponse(&authv1.UnbindOauthIdentityResponse{}), nil
}
