package rpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
)

func (s *SystemService) smsOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "sms",
		load:     s.settings.Sms,
		save:     s.settings.SetSms,
		purposes: sms.Purposes,
		keepSecrets: func(updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
			return mergeProfile(sms.Registry, updated, stored)
		},
		validate: func(profile kitsettings.GenericProfile) error {
			return validateProfile("sms", sms.Registry, profile)
		},
	}
}

func mergeProfile[Ops any](registry integration.Registry[Ops], updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
	if merged, ok := registry.Merge(updated.Provider, updated.Config, stored.Config); ok {
		updated.Config = merged
	}
	return updated
}

func validateProfile[Ops any](name string, registry integration.Registry[Ops], profile kitsettings.GenericProfile) error {
	if err := registry.Validate(profile.Provider, profile.Config); err != nil {
		return fmt.Errorf("invalid %s profile: %w", name, err)
	}
	return nil
}

func describeProviders[Ops any](registry integration.Registry[Ops]) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	providers := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, descriptor := range descriptors {
		js, ui := descriptor.Config.JSONForm()
		providers[i] = &systemv1.ProviderDescriptor{
			Key:          descriptor.Key,
			Presentation: presentationToProto(descriptor.Presentation),
			Config:       &systemv1.ProviderForm{Schema: toStruct(js), UiSchema: toStruct(ui)},
		}
	}
	return providers
}

func describePurposes(catalog integration.Catalog) []*systemv1.PurposeDescriptor {
	out := make([]*systemv1.PurposeDescriptor, len(catalog))
	for i, purpose := range catalog {
		cardinality := systemv1.BindingCardinality_BINDING_CARDINALITY_SINGLE
		if purpose.Cardinality == integration.Multiple {
			cardinality = systemv1.BindingCardinality_BINDING_CARDINALITY_MULTIPLE
		}
		out[i] = &systemv1.PurposeDescriptor{
			Key: purpose.Key,
			Presentation: &systemv1.Presentation{
				Name: purpose.Name, Description: purpose.Description,
			},
			Cardinality: cardinality,
		}
	}
	return out
}

func presentationToProto(presentation integration.Presentation) *systemv1.Presentation {
	return &systemv1.Presentation{
		Name: presentation.Name, Description: presentation.Description,
		Color: presentation.Color, IconRef: presentation.IconRef,
	}
}

func (s *SystemService) CreateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateSmsProfileRequest],
) (*connect.Response[systemv1.CreateSmsProfileResponse], error) {
	profile, err := s.smsOps().create(ctx, s, profileFromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateSmsProfileResponse{
		Profile: smsProfileToProto(profile),
	}), nil
}

func (s *SystemService) UpdateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateSmsProfileRequest],
) (*connect.Response[systemv1.UpdateSmsProfileResponse], error) {
	profile, err := s.smsOps().update(ctx, s, profileFromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateSmsProfileResponse{
		Profile: smsProfileToProto(profile),
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
	var in *kitsettings.GenericProfile
	if req.Msg.GetProfile() != nil {
		p := profileFromProto(req.Msg.GetProfile())
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
	if err := s.smser.SendCodeWith(ctx, profile.Provider, profile.Config, e164, "123456"); err != nil {
		return connect.NewResponse(&systemv1.SendTestSmsResponse{
			Ok:      false,
			Message: testFailureMessage(err, sms.ErrNotConfigured, "sms is not configured: fill in provider credentials and sign name"),
		}), nil
	}
	return connect.NewResponse(&systemv1.SendTestSmsResponse{Ok: true}), nil
}

func (s *SystemService) DescribeSmsProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeSmsProvidersRequest],
) (*connect.Response[systemv1.DescribeSmsProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeSmsProvidersResponse{
		Purposes:  describePurposes(sms.Purposes),
		Providers: describeProviders(sms.Registry),
	}), nil
}

func profileFromProto(p *systemv1.Profile) kitsettings.GenericProfile {
	var config map[string]any
	if c := p.GetConfig(); c != nil {
		config = c.AsMap()
	}
	return kitsettings.GenericProfile{
		Id:       p.GetId(),
		Name:     p.GetName(),
		Provider: p.GetProvider(),
		Config:   config,
	}
}

func smsProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	return profileToProto(p, sms.Registry)
}

// profileToProto masks the config via the provider's schema — secrets never
// leave the server — then encodes it as a Struct. The masked map holds only
// strings and *_set bools, so structpb encoding cannot fail.
func profileToProto[Ops any](p kitsettings.GenericProfile, registry integration.Registry[Ops]) *systemv1.Profile {
	masked, _ := registry.Mask(p.Provider, p.Config)
	cfg, err := structpb.NewStruct(masked)
	if err != nil {
		cfg = &structpb.Struct{}
	}
	return &systemv1.Profile{Id: p.Id, Name: p.Name, Provider: p.Provider, Config: cfg}
}

// toStruct encodes a JSONForm map (only strings, numbers, bools, maps and
// slices) as a Struct; encoding cannot fail, so an empty Struct is a safe
// fallback that keeps callers error-free.
func toStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		return &structpb.Struct{}
	}
	return s
}

func toProtoSms(cfg settings.Sms) *systemv1.SmsSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = smsProfileToProto(p)
	}
	bindings := make([]*systemv1.SmsBinding, len(sms.Purposes))
	for i, purpose := range sms.Purposes {
		bindings[i] = &systemv1.SmsBinding{
			Purpose:   purpose.Key,
			ProfileId: firstID(cfg.Bindings[purpose.Key]),
		}
	}
	return &systemv1.SmsSettings{Profiles: profiles, Bindings: bindings}
}
