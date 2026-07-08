package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
)

func (s *SystemService) smsOps() integrationOps[systemcodec.SmsProfile] {
	return integrationOps[systemcodec.SmsProfile]{
		name:        "sms",
		load:        s.settings.Sms,
		save:        s.settings.SetSms,
		purposes:    sms.Purposes,
		keepSecrets: systemcodec.SmsCodec.Merge,
	}
}

func (s *SystemService) CreateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateSmsProfileRequest],
) (*connect.Response[systemv1.CreateSmsProfileResponse], error) {
	profile, err := s.smsOps().create(ctx, s, systemcodec.SmsCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateSmsProfileResponse{
		Profile: systemcodec.SmsCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateSmsProfileRequest],
) (*connect.Response[systemv1.UpdateSmsProfileResponse], error) {
	profile, err := s.smsOps().update(ctx, s, systemcodec.SmsCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateSmsProfileResponse{
		Profile: systemcodec.SmsCodec.Mask(profile),
	}), nil
}

func (s *SystemService) DeleteSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteSmsProfileRequest],
) (*connect.Response[systemv1.DeleteSmsProfileResponse], error) {
	if err := s.smsOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteSmsProfileResponse{}), nil
}

func (s *SystemService) BindSmsPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindSmsPurposeRequest],
) (*connect.Response[systemv1.BindSmsPurposeResponse], error) {
	cfg, err := s.smsOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindSmsPurposeResponse{
		Sms: toProtoSms(cfg),
	}), nil
}

func (s *SystemService) SendTestSms(
	ctx context.Context,
	req *connect.Request[systemv1.SendTestSmsRequest],
) (*connect.Response[systemv1.SendTestSmsResponse], error) {
	var in *systemcodec.SmsProfile
	if req.Msg.GetProfile() != nil {
		p := systemcodec.SmsCodec.FromProto(req.Msg.GetProfile())
		in = &p
	}
	profile, err := s.smsOps().resolveTestProfile(ctx, s, in, req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	e164, _, err := phone.Normalize(req.Msg.GetPhoneNumber())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, phone.ErrInvalid)
	}
	if err := s.smser.SendCodeWith(ctx, profile, e164, "123456"); err != nil {
		return connect.NewResponse(&systemv1.SendTestSmsResponse{
			Ok:      false,
			Message: testFailureMessage(err, sms.ErrNotConfigured, "sms is not configured: fill in provider credentials and sign name"),
		}), nil
	}
	return connect.NewResponse(&systemv1.SendTestSmsResponse{Ok: true}), nil
}

func toProtoSms(cfg settings.Sms) *systemv1.SmsSettings {
	profiles := make([]*systemv1.SmsProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.SmsCodec.Mask(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.SmsBinding, len(sms.Purposes))
	for i, purpose := range sms.Purposes {
		bindings[i] = &systemv1.SmsBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.SmsSettings{Profiles: profiles, Bindings: bindings}
}
