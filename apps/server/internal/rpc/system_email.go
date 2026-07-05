package rpc

import (
	"context"

	"connectrpc.com/connect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

func (s *SystemService) emailOps() channelOps[systemcodec.EmailProfile] {
	return channelOps[systemcodec.EmailProfile]{
		name:        "email",
		load:        s.settings.Email,
		save:        s.settings.SetEmail,
		purposes:    mail.Purposes,
		keepSecrets: systemcodec.EmailCodec.Merge,
	}
}

func (s *SystemService) CreateEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateEmailProfileRequest],
) (*connect.Response[systemv1.CreateEmailProfileResponse], error) {
	profile, err := s.emailOps().create(ctx, s, systemcodec.EmailCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateEmailProfileResponse{
		Profile: systemcodec.EmailCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdateEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateEmailProfileRequest],
) (*connect.Response[systemv1.UpdateEmailProfileResponse], error) {
	profile, err := s.emailOps().update(ctx, s, systemcodec.EmailCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateEmailProfileResponse{
		Profile: systemcodec.EmailCodec.Mask(profile),
	}), nil
}

func (s *SystemService) DeleteEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteEmailProfileRequest],
) (*connect.Response[systemv1.DeleteEmailProfileResponse], error) {
	if err := s.emailOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteEmailProfileResponse{}), nil
}

func (s *SystemService) BindEmailPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindEmailPurposeRequest],
) (*connect.Response[systemv1.BindEmailPurposeResponse], error) {
	cfg, err := s.emailOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindEmailPurposeResponse{
		Email: toProtoEmail(cfg),
	}), nil
}

func (s *SystemService) SendTestEmail(
	ctx context.Context,
	req *connect.Request[systemv1.SendTestEmailRequest],
) (*connect.Response[systemv1.SendTestEmailResponse], error) {
	var in *systemcodec.EmailProfile
	if req.Msg.GetProfile() != nil {
		p := systemcodec.EmailCodec.FromProto(req.Msg.GetProfile())
		in = &p
	}
	profile, err := s.emailOps().resolveTestProfile(ctx, s, in, req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	err = s.mailer.SendWith(ctx, profile, req.Msg.GetTo(),
		"Test email", "This is a test email from your admin panel. Delivery is working.")
	if err != nil {
		return connect.NewResponse(&systemv1.SendTestEmailResponse{
			Ok:      false,
			Message: testFailureMessage(err, mail.ErrNotConfigured, "email is not configured: fill in the delivery settings and from address"),
		}), nil
	}
	return connect.NewResponse(&systemv1.SendTestEmailResponse{Ok: true}), nil
}

func toProtoEmail(cfg settings.Email) *systemv1.EmailSettings {
	profiles := make([]*systemv1.EmailProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.EmailCodec.Mask(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.EmailBinding, len(mail.Purposes))
	for i, purpose := range mail.Purposes {
		bindings[i] = &systemv1.EmailBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.EmailSettings{Profiles: profiles, Bindings: bindings}
}
