package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/sms"
)

// systemSms is the sms integration's admin surface: profile CRUD, purpose
// binding, test send and provider descriptors. It owns only the sms registry
// and sender on top of the shared systemBase.
type systemSms struct {
	systemBase
	smsRegistry smsint.Registry
	smser       sms.ProfileSender
}

func (s *systemSms) smsOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "sms",
		load:     s.settings.Sms,
		save:     s.settings.SetSms,
		purposes: sms.Purposes,
	}
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

func (s *systemSms) CreateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateSmsProfileRequest],
) (*connect.Response[systemv1.CreateSmsProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Sms(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load sms settings", err)
	}
	canonical, err := s.smsRegistry.CreateConfig(input.GetProvider(), configValues(input.GetConfig()), input.GetConfig().GetSecrets())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	profile := kitsettings.GenericProfile{Id: uuid.NewString(), Name: input.GetName(), Provider: input.GetProvider(), Config: canonical}
	cfg.Profiles = append(cfg.Profiles, profile)
	if err := s.settings.SetSms(ctx, cfg); err != nil {
		return nil, s.internal(ctx, "save sms settings", err)
	}
	return connect.NewResponse(&systemv1.CreateSmsProfileResponse{
		Profile: s.smsProfileToProto(profile),
	}), nil
}

func (s *systemSms) UpdateSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateSmsProfileRequest],
) (*connect.Response[systemv1.UpdateSmsProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Sms(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load sms settings", err)
	}
	for i, stored := range cfg.Profiles {
		if stored.Id != input.GetId() {
			continue
		}
		if stored.Provider != input.GetProvider() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("sms provider cannot be changed"))
		}
		canonical, err := s.smsRegistry.UpdateConfig(stored.Provider, configValues(input.GetConfig()), input.GetConfig().GetSecrets(), stored.Config)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		profile := kitsettings.GenericProfile{Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical}
		cfg.Profiles[i] = profile
		if err := s.settings.SetSms(ctx, cfg); err != nil {
			return nil, s.internal(ctx, "save sms settings", err)
		}
		return connect.NewResponse(&systemv1.UpdateSmsProfileResponse{Profile: s.smsProfileToProto(profile)}), nil
	}
	return nil, s.smsOps().errNotFound()
}

func (s *systemSms) DeleteSmsProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteSmsProfileRequest],
) (*connect.Response[systemv1.DeleteSmsProfileResponse], error) {
	if err := s.smsOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteSmsProfileResponse{}), nil
}

func (s *systemSms) BindSmsPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindSmsPurposeRequest],
) (*connect.Response[systemv1.BindSmsPurposeResponse], error) {
	cfg, err := s.smsOps().bind(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindSmsPurposeResponse{
		Sms: s.toProtoSms(cfg),
	}), nil
}

func (s *systemSms) SendTestSms(
	ctx context.Context,
	req *connect.Request[systemv1.SendTestSmsRequest],
) (*connect.Response[systemv1.SendTestSmsResponse], error) {
	profile, err := s.resolveSmsTestProfile(ctx, req.Msg.GetProfile(), req.Msg.GetProfileId())
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

func (s *systemSms) resolveSmsTestProfile(ctx context.Context, input *systemv1.ProfileInput, id string) (kitsettings.GenericProfile, error) {
	cfg, err := s.settings.Sms(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, s.internal(ctx, "load sms settings", err)
	}
	if input != nil {
		if stored, ok := cfg.Profile(input.GetId()); ok {
			if stored.Provider != input.GetProvider() {
				return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, errors.New("sms provider cannot be changed"))
			}
			canonical, err := s.smsRegistry.UpdateConfig(stored.Provider, configValues(input.GetConfig()), input.GetConfig().GetSecrets(), stored.Config)
			if err != nil {
				return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, err)
			}
			return kitsettings.GenericProfile{Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical}, nil
		}
		canonical, err := s.smsRegistry.CreateConfig(input.GetProvider(), configValues(input.GetConfig()), input.GetConfig().GetSecrets())
		if err != nil {
			return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return kitsettings.GenericProfile{Id: input.GetId(), Name: input.GetName(), Provider: input.GetProvider(), Config: canonical}, nil
	}
	if id != "" {
		if stored, ok := cfg.Profile(id); ok {
			return stored, nil
		}
		return kitsettings.GenericProfile{}, s.smsOps().errNotFound()
	}
	return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, errors.New("provide a profile or profile_id to test"))
}

func (s *systemSms) DescribeSmsProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeSmsProvidersRequest],
) (*connect.Response[systemv1.DescribeSmsProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeSmsProvidersResponse{
		Purposes:  describePurposes(sms.Purposes),
		Providers: describeSmsProviders(s.smsRegistry),
	}), nil
}

func describeSmsProviders(registry smsint.Registry) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, d := range descriptors {
		out[i] = &systemv1.ProviderDescriptor{Key: d.Key, Presentation: presentationToProto(d.Presentation), Config: &systemv1.ProviderForm{Schema: toStruct(d.JSONSchema), UiSchema: toStruct(d.UISchema)}}
	}
	return out
}

func (s *systemSms) smsProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.smsRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{Id: p.Id, Name: p.Name, Provider: p.Provider, Config: &systemv1.ConfigView{Values: toStruct(view.Values), SetSecretPaths: view.SetSecretPaths}, ConfigValid: valid}
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

func (s *systemSms) smsSnapshot(ctx context.Context) (*systemv1.SmsSettings, error) {
	cfg, err := s.settings.Sms(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load sms settings", err)
	}
	return s.toProtoSms(cfg), nil
}

func (s *systemSms) toProtoSms(cfg settings.Sms) *systemv1.SmsSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.smsProfileToProto(p)
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
