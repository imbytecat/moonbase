package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) paymentOps() integrationOps[systemcodec.PaymentProfile] {
	return integrationOps[systemcodec.PaymentProfile]{
		name:        "payment",
		load:        s.settings.Payment,
		save:        s.settings.SetPayment,
		purposes:    pay.Purposes,
		keepSecrets: systemcodec.PaymentCodec.Merge,
	}
}

// validatePaymentMethods rejects a profile whose signed products aren't in its
// provider's catalog. It is the save-time guard that replaces the removed proto
// `in:` rule on PaymentProfile.methods (an empty list is valid — "all products").
func validatePaymentMethods(p systemcodec.PaymentProfile) error {
	if err := pay.ValidateMethods(p.Provider, p.Methods); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return nil
}

func (s *SystemService) CreatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreatePaymentProfileRequest],
) (*connect.Response[systemv1.CreatePaymentProfileResponse], error) {
	in := systemcodec.PaymentCodec.FromProto(req.Msg.GetProfile())
	if err := validatePaymentMethods(in); err != nil {
		return nil, err
	}
	profile, err := s.paymentOps().create(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreatePaymentProfileResponse{
		Profile: systemcodec.PaymentCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdatePaymentProfileRequest],
) (*connect.Response[systemv1.UpdatePaymentProfileResponse], error) {
	in := systemcodec.PaymentCodec.FromProto(req.Msg.GetProfile())
	if err := validatePaymentMethods(in); err != nil {
		return nil, err
	}
	profile, err := s.paymentOps().update(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdatePaymentProfileResponse{
		Profile: systemcodec.PaymentCodec.Mask(profile),
	}), nil
}

func (s *SystemService) DeletePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeletePaymentProfileRequest],
) (*connect.Response[systemv1.DeletePaymentProfileResponse], error) {
	if err := s.paymentOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeletePaymentProfileResponse{}), nil
}

func (s *SystemService) BindPaymentPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindPaymentPurposeRequest],
) (*connect.Response[systemv1.BindPaymentPurposeResponse], error) {
	cfg, err := s.paymentOps().bindMany(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileIds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindPaymentPurposeResponse{
		Payment: toProtoPayment(cfg),
	}), nil
}

func toProtoPayment(cfg settings.Payment) *systemv1.PaymentSettings {
	profiles := make([]*systemv1.PaymentProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.PaymentCodec.Mask(p)
	}
	bindings := make([]*systemv1.PaymentBinding, len(pay.Purposes))
	for i, purpose := range pay.Purposes {
		bindings[i] = &systemv1.PaymentBinding{
			Purpose:    purpose,
			ProfileIds: cfg.Bindings[purpose],
		}
	}
	return &systemv1.PaymentSettings{Profiles: profiles, Bindings: bindings}
}
