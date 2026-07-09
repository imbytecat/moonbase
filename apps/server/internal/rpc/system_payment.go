package rpc

import (
	"context"

	"connectrpc.com/connect"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) paymentOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "payment",
		load:     s.settings.Payment,
		save:     s.settings.SetPayment,
		purposes: pay.Purposes,
		keepSecrets: func(updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
			return mergeProfile(pay.Schemas(), updated, stored)
		},
		validate: paymentValidate,
	}
}

// validatePaymentMethods rejects a profile whose signed products aren't in its
// provider's catalog. It is the save-time guard for Profile.config.methods (an
// empty list is valid — "all products").
func paymentValidate(p kitsettings.GenericProfile) error {
	if err := validateProfile("payment", pay.Schemas(), p); err != nil {
		return err
	}
	return pay.ValidateMethods(p.Provider, pay.ProfileMethods(p))
}

func (s *SystemService) CreatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreatePaymentProfileRequest],
) (*connect.Response[systemv1.CreatePaymentProfileResponse], error) {
	in := profileFromProto(req.Msg.GetProfile())
	profile, err := s.paymentOps().create(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreatePaymentProfileResponse{
		Profile: paymentProfileToProto(profile),
	}), nil
}

func (s *SystemService) UpdatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdatePaymentProfileRequest],
) (*connect.Response[systemv1.UpdatePaymentProfileResponse], error) {
	in := profileFromProto(req.Msg.GetProfile())
	profile, err := s.paymentOps().update(ctx, s, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdatePaymentProfileResponse{
		Profile: paymentProfileToProto(profile),
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
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = paymentProfileToProto(p)
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

func (s *SystemService) DescribePaymentProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribePaymentProvidersRequest],
) (*connect.Response[systemv1.DescribePaymentProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribePaymentProvidersResponse{Providers: describeProviders(pay.Schemas())}), nil
}

func paymentProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	return profileToProto(p, pay.Schemas()[p.Provider])
}
