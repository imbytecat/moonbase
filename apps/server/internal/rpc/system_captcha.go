package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/imbytecat/moonbase/packages/integrations/captcha"
	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) captchaOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "captcha",
		load:     s.settings.Captcha,
		save:     s.settings.SetCaptcha,
		purposes: captcha.Purposes,
		keepSecrets: func(updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
			return mergeProfile(captcha.Schemas(), updated, stored)
		},
		validate: func(p kitsettings.GenericProfile) error { return validateProfile("captcha", captcha.Schemas(), p) },
	}
}

func (s *SystemService) CreateCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateCaptchaProfileRequest],
) (*connect.Response[systemv1.CreateCaptchaProfileResponse], error) {
	profile, err := s.captchaOps().create(ctx, s, profileFromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateCaptchaProfileResponse{
		Profile: captchaProfileToProto(profile),
	}), nil
}

func (s *SystemService) UpdateCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateCaptchaProfileRequest],
) (*connect.Response[systemv1.UpdateCaptchaProfileResponse], error) {
	profile, err := s.captchaOps().update(ctx, s, profileFromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateCaptchaProfileResponse{
		Profile: captchaProfileToProto(profile),
	}), nil
}

func (s *SystemService) DeleteCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteCaptchaProfileRequest],
) (*connect.Response[systemv1.DeleteCaptchaProfileResponse], error) {
	if err := s.captchaOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteCaptchaProfileResponse{}), nil
}

func (s *SystemService) BindCaptchaPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindCaptchaPurposeRequest],
) (*connect.Response[systemv1.BindCaptchaPurposeResponse], error) {
	cfg, err := s.captchaOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindCaptchaPurposeResponse{
		Captcha: toProtoCaptcha(cfg),
	}), nil
}

func toProtoCaptcha(cfg settings.Captcha) *systemv1.CaptchaSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = captchaProfileToProto(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.CaptchaBinding, len(captcha.Purposes))
	for i, purpose := range captcha.Purposes {
		bindings[i] = &systemv1.CaptchaBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.CaptchaSettings{Profiles: profiles, Bindings: bindings}
}

func (s *SystemService) DescribeCaptchaProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeCaptchaProvidersRequest],
) (*connect.Response[systemv1.DescribeCaptchaProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeCaptchaProvidersResponse{Providers: describeProviders(captcha.Schemas())}), nil
}

func captchaProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	return profileToProto(p, captcha.Schemas()[p.Provider])
}
