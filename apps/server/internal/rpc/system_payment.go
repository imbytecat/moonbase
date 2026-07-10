package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	paymentint "github.com/imbytecat/moonbase/integrations/payment"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// systemPayment is the payment integration's admin surface: profile CRUD (with
// product validation), purpose binding and provider descriptors. It owns only
// the payment registry on top of the shared systemBase.
type systemPayment struct {
	systemBase
	paymentRegistry paymentint.Registry
}

func (s *systemPayment) paymentOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "payment",
		load:     s.settings.Payment,
		save:     s.settings.SetPayment,
		purposes: pay.Purposes,
	}
}

func (s *systemPayment) CreatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreatePaymentProfileRequest],
) (*connect.Response[systemv1.CreatePaymentProfileResponse], error) {
	input := req.Msg.GetProfile()
	settings, err := s.settings.Payment(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load payment settings", err)
	}
	canonical, err := s.paymentRegistry.CreateConfig(input.GetProvider(), configValues(input.GetConfig()), input.GetConfig().GetSecrets())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.paymentRegistry.ValidateProducts(input.GetProvider(), paymentProducts(canonical)); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	profile := kitsettings.GenericProfile{Id: uuid.NewString(), Name: input.GetName(), Provider: input.GetProvider(), Config: canonical}
	settings.Profiles = append(settings.Profiles, profile)
	if err := s.settings.SetPayment(ctx, settings); err != nil {
		return nil, s.internal(ctx, "save payment settings", err)
	}
	return connect.NewResponse(&systemv1.CreatePaymentProfileResponse{
		Profile: s.paymentProfileToProto(profile),
	}), nil
}

func (s *systemPayment) UpdatePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdatePaymentProfileRequest],
) (*connect.Response[systemv1.UpdatePaymentProfileResponse], error) {
	input := req.Msg.GetProfile()
	settings, err := s.settings.Payment(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load payment settings", err)
	}
	for i, stored := range settings.Profiles {
		if stored.Id != input.GetId() {
			continue
		}
		if stored.Provider != input.GetProvider() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("payment provider cannot be changed"))
		}
		canonical, err := s.paymentRegistry.UpdateConfig(stored.Provider, configValues(input.GetConfig()), input.GetConfig().GetSecrets(), stored.Config)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if err := s.paymentRegistry.ValidateProducts(stored.Provider, paymentProducts(canonical)); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		profile := kitsettings.GenericProfile{Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical}
		settings.Profiles[i] = profile
		if err := s.settings.SetPayment(ctx, settings); err != nil {
			return nil, s.internal(ctx, "save payment settings", err)
		}
		return connect.NewResponse(&systemv1.UpdatePaymentProfileResponse{Profile: s.paymentProfileToProto(profile)}), nil
	}
	return nil, s.paymentOps().errNotFound()
}

func paymentProducts(values map[string]any) []string {
	products, _ := values["products"].([]any)
	out := make([]string, 0, len(products))
	for _, product := range products {
		if value, ok := product.(string); ok {
			out = append(out, value)
		}
	}
	return out
}

func (s *systemPayment) DeletePaymentProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeletePaymentProfileRequest],
) (*connect.Response[systemv1.DeletePaymentProfileResponse], error) {
	if err := s.paymentOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeletePaymentProfileResponse{}), nil
}

func (s *systemPayment) BindPaymentPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindPaymentPurposeRequest],
) (*connect.Response[systemv1.BindPaymentPurposeResponse], error) {
	cfg, err := s.paymentOps().bindMany(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileIds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindPaymentPurposeResponse{
		Payment: s.toProtoPayment(cfg),
	}), nil
}

func (s *systemPayment) paymentSnapshot(ctx context.Context) (*systemv1.PaymentSettings, error) {
	cfg, err := s.settings.Payment(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load payment settings", err)
	}
	return s.toProtoPayment(cfg), nil
}

func (s *systemPayment) toProtoPayment(cfg settings.Payment) *systemv1.PaymentSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.paymentProfileToProto(p)
	}
	bindings := make([]*systemv1.PaymentBinding, len(pay.Purposes))
	for i, purpose := range pay.Purposes {
		bindings[i] = &systemv1.PaymentBinding{
			Purpose:    purpose.Key,
			ProfileIds: cfg.Bindings[purpose.Key],
		}
	}
	return &systemv1.PaymentSettings{Profiles: profiles, Bindings: bindings}
}

func (s *systemPayment) DescribePaymentProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribePaymentProvidersRequest],
) (*connect.Response[systemv1.DescribePaymentProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribePaymentProvidersResponse{
		Purposes: describePurposes(pay.Purposes), Providers: describePaymentProviders(s.paymentRegistry),
	}), nil
}

func describePaymentProviders(registry paymentint.Registry) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	providers := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, descriptor := range descriptors {
		provider := &systemv1.ProviderDescriptor{
			Key: descriptor.Key, Presentation: presentationToProto(descriptor.Presentation),
			Config: &systemv1.ProviderForm{Schema: toStruct(descriptor.ConfigSchema), UiSchema: toStruct(descriptor.UISchema)},
		}
		payment := &systemv1.PaymentProviderDescriptor{Capabilities: descriptor.Payment.Capabilities}
		for _, method := range descriptor.Payment.Methods {
			payment.Methods = append(payment.Methods, &systemv1.PaymentMethodDescriptor{
				Key: method.Key, Presentation: presentationToProto(method.Presentation),
			})
		}
		for _, product := range descriptor.Payment.Products {
			js, ui := product.Input.JSONForm()
			payment.Products = append(payment.Products, &systemv1.PaymentProductDescriptor{
				Id: product.ID, PaymentMethod: product.Method,
				Presentation: presentationToProto(product.Presentation),
				Input:        &systemv1.ProviderForm{Schema: toStruct(js), UiSchema: toStruct(ui)},
			})
		}
		provider.Payment = payment
		providers[i] = provider
	}
	return providers
}

func (s *systemPayment) paymentProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.paymentRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{
		Id: p.Id, Name: p.Name, Provider: p.Provider,
		Config:      &systemv1.ConfigView{Values: toStruct(view.Values), SetSecretPaths: view.SetSecretPaths},
		ConfigValid: valid,
	}
}
