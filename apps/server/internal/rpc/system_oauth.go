package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/integrations/oauth"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) oauthOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "login provider",
		load:     s.settings.Oauth,
		save:     s.settings.SetOauth,
		purposes: oauth.Purposes,
		keepSecrets: func(updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
			return mergeProfile(oauth.Schemas(), updated, stored)
		},
		validate: func(p kitsettings.GenericProfile) error { return validateProfile("oauth", oauth.Schemas(), p) },
	}
}

func (s *SystemService) CreateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateOauthProfileRequest],
) (*connect.Response[systemv1.CreateOauthProfileResponse], error) {
	in := profileFromProto(req.Msg.GetProfile())
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	key := profileConfigString(in, "key")
	if _, exists := settings.ProfileByKey(cfg, key); exists {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a login provider with key %q already exists", key))
	}
	profile, err := s.oauthOps().create(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateOauthProfileResponse{
		Profile: oauthProfileToProto(profile),
	}), nil
}

func (s *SystemService) UpdateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateOauthProfileRequest],
) (*connect.Response[systemv1.UpdateOauthProfileResponse], error) {
	profile, err := s.oauthOps().update(ctx, s, profileFromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateOauthProfileResponse{
		Profile: oauthProfileToProto(profile),
	}), nil
}

func (s *SystemService) BindOauthPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindOauthPurposeRequest],
) (*connect.Response[systemv1.BindOauthPurposeResponse], error) {
	cfg, err := s.oauthOps().bindMany(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileIds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindOauthPurposeResponse{
		Oauth: toProtoOauth(cfg),
	}), nil
}

func (s *SystemService) DeleteOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteOauthProfileRequest],
) (*connect.Response[systemv1.DeleteOauthProfileResponse], error) {
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	profile, ok := cfg.Profile(req.Msg.GetId())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("login provider not found"))
	}
	// Bound identities would become unreachable orphans; unbinding the
	// login purpose is the reversible way to retire a provider.
	count, err := s.repo.CountIdentitiesByProvider(ctx, profileConfigString(profile, "key"))
	if err != nil {
		return nil, s.internal(ctx, "count identities", err)
	}
	if count > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("%d account(s) still sign in through this provider — unbind it from the sign-in page instead", count))
	}
	if err := s.oauthOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteOauthProfileResponse{}), nil
}

func toProtoOauth(cfg settings.OAuth) *systemv1.OauthSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = oauthProfileToProto(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.OauthBinding, len(oauth.Purposes))
	for i, purpose := range oauth.Purposes {
		bindings[i] = &systemv1.OauthBinding{
			Purpose:    purpose,
			ProfileIds: cfg.Bindings[purpose],
		}
	}
	return &systemv1.OauthSettings{Profiles: profiles, Bindings: bindings}
}

func (s *SystemService) DescribeOauthProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeOauthProvidersRequest],
) (*connect.Response[systemv1.DescribeOauthProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeOauthProvidersResponse{Providers: describeProviders(oauth.Schemas())}), nil
}

func oauthProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	return profileToProto(p, oauth.Schemas()[p.Provider])
}

func profileConfigString(p kitsettings.GenericProfile, key string) string {
	s, _ := p.Config[key].(string)
	return s
}
