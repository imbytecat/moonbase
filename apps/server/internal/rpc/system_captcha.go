package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/server/internal/captcha"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// systemCaptcha is the captcha integration's admin surface: profile CRUD,
// purpose binding and provider descriptors. It owns only the captcha registry
// on top of the shared systemBase.
type systemCaptcha struct {
	systemBase
	captchaRegistry captchaint.Registry
}

func (s *systemCaptcha) captchaOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "captcha",
		load:     s.settings.Captcha,
		save:     s.settings.SetCaptcha,
		purposes: captcha.Purposes,
	}
}

func (s *systemCaptcha) CreateCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateCaptchaProfileRequest],
) (*connect.Response[systemv1.CreateCaptchaProfileResponse], error) {
	in := req.Msg.GetProfile()
	cfg, err := s.settings.Captcha(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load captcha settings", err)
	}
	canonical, err := s.captchaRegistry.CreateConfig(
		in.GetProvider(),
		configValues(in.GetConfig()),
		in.GetConfig().GetSecrets(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	p := kitsettings.GenericProfile{
		Id:       uuid.NewString(),
		Name:     in.GetName(),
		Provider: in.GetProvider(),
		Config:   canonical,
	}
	cfg.Profiles = append(cfg.Profiles, p)
	if err := s.settings.SetCaptcha(ctx, cfg); err != nil {
		return nil, s.internal(ctx, "save captcha settings", err)
	}
	return connect.NewResponse(
		&systemv1.CreateCaptchaProfileResponse{Profile: s.captchaProfileToProto(p)},
	), nil
}

func (s *systemCaptcha) UpdateCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateCaptchaProfileRequest],
) (*connect.Response[systemv1.UpdateCaptchaProfileResponse], error) {
	in := req.Msg.GetProfile()
	cfg, err := s.settings.Captcha(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load captcha settings", err)
	}
	for i, old := range cfg.Profiles {
		if old.Id != in.GetId() {
			continue
		}
		if old.Provider != in.GetProvider() {
			return nil, connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("captcha provider cannot be changed"),
			)
		}
		canonical, err := s.captchaRegistry.UpdateConfig(
			old.Provider,
			configValues(in.GetConfig()),
			in.GetConfig().GetSecrets(),
			old.Config,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		p := kitsettings.GenericProfile{
			Id:       old.Id,
			Name:     in.GetName(),
			Provider: old.Provider,
			Config:   canonical,
		}
		cfg.Profiles[i] = p
		if err := s.settings.SetCaptcha(ctx, cfg); err != nil {
			return nil, s.internal(ctx, "save captcha settings", err)
		}
		return connect.NewResponse(
			&systemv1.UpdateCaptchaProfileResponse{Profile: s.captchaProfileToProto(p)},
		), nil
	}
	return nil, s.captchaOps().errNotFound()
}

func (s *systemCaptcha) DeleteCaptchaProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteCaptchaProfileRequest],
) (*connect.Response[systemv1.DeleteCaptchaProfileResponse], error) {
	if err := s.captchaOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteCaptchaProfileResponse{}), nil
}

func (s *systemCaptcha) BindCaptchaPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindCaptchaPurposeRequest],
) (*connect.Response[systemv1.BindCaptchaPurposeResponse], error) {
	cfg, err := s.captchaOps().
		bind(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(
		&systemv1.BindCaptchaPurposeResponse{Captcha: s.toProtoCaptcha(cfg)},
	), nil
}
func (s *systemCaptcha) captchaSnapshot(ctx context.Context) (*systemv1.CaptchaSettings, error) {
	cfg, err := s.settings.Captcha(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load captcha settings", err)
	}
	return s.toProtoCaptcha(cfg), nil
}
func (s *systemCaptcha) toProtoCaptcha(cfg settings.Captcha) *systemv1.CaptchaSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.captchaProfileToProto(p)
	}
	bindings := make([]*systemv1.CaptchaBinding, len(captcha.Purposes))
	for i, p := range captcha.Purposes {
		bindings[i] = &systemv1.CaptchaBinding{
			Purpose:   p.Key,
			ProfileId: firstID(cfg.Bindings[p.Key]),
		}
	}
	return &systemv1.CaptchaSettings{Profiles: profiles, Bindings: bindings}
}

func (s *systemCaptcha) DescribeCaptchaProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeCaptchaProvidersRequest],
) (*connect.Response[systemv1.DescribeCaptchaProvidersResponse], error) {
	return connect.NewResponse(
		&systemv1.DescribeCaptchaProvidersResponse{
			Purposes:  describePurposes(captcha.Purposes),
			Providers: describeCaptchaProviders(s.captchaRegistry),
		},
	), nil
}
func describeCaptchaProviders(r captchaint.Registry) []*systemv1.ProviderDescriptor {
	ds := r.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(ds))
	for i, d := range ds {
		out[i] = &systemv1.ProviderDescriptor{
			Key:          d.Key,
			Presentation: presentationToProto(d.Presentation),
			Config: &systemv1.ProviderForm{
				Schema:   toStruct(d.JSONSchema),
				UiSchema: toStruct(d.UISchema),
			},
		}
	}
	return out
}
func (s *systemCaptcha) captchaProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.captchaRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{
		Id:       p.Id,
		Name:     p.Name,
		Provider: p.Provider,
		Config: &systemv1.ConfigView{
			Values:         toStruct(view.Values),
			SetSecretPaths: view.SetSecretPaths,
		},
		ConfigValid: valid,
	}
}
