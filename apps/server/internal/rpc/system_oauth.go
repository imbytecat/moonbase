package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

func (s *SystemService) oauthOps() channelOps[systemcodec.OauthProfile] {
	return channelOps[systemcodec.OauthProfile]{
		name:        "login provider",
		load:        s.settings.Oauth,
		save:        s.settings.SetOauth,
		purposes:    oauth.Purposes,
		keepSecrets: systemcodec.OauthCodec.Merge,
	}
}

func (s *SystemService) CreateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateOauthProfileRequest],
) (*connect.Response[systemv1.CreateOauthProfileResponse], error) {
	in := systemcodec.OauthCodec.FromProto(req.Msg.GetProfile())
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	if _, exists := settings.ProfileByKey(cfg, in.Key); exists {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a login provider with key %q already exists", in.Key))
	}
	profile, err := s.oauthOps().create(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateOauthProfileResponse{
		Profile: systemcodec.OauthCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateOauthProfileRequest],
) (*connect.Response[systemv1.UpdateOauthProfileResponse], error) {
	profile, err := s.oauthOps().update(ctx, s, systemcodec.OauthCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateOauthProfileResponse{
		Profile: systemcodec.OauthCodec.Mask(profile),
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
	count, err := s.repo.CountIdentitiesByProvider(ctx, profile.Key)
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
	profiles := make([]*systemv1.OauthProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.OauthCodec.Mask(p)
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
