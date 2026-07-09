package rpc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/imbytecat/moonbase/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/integrations/sms"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) smsOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:        "sms",
		load:        s.settings.Sms,
		save:        s.settings.SetSms,
		purposes:    sms.Purposes,
		keepSecrets: smsMerge,
		validate:    smsValidate,
	}
}

func smsMerge(updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
	return mergeProfile(sms.Schemas(), updated, stored)
}

func smsValidate(p kitsettings.GenericProfile) error {
	return validateProfile("sms", sms.Schemas(), p)
}

func mergeProfile(schemas map[string]schema.Schema, updated, stored kitsettings.GenericProfile) kitsettings.GenericProfile {
	if sch, ok := schemas[updated.Provider]; ok {
		updated.Config = sch.Merge(updated.Config, stored.Config)
	}
	return updated
}

func validateProfile(name string, schemas map[string]schema.Schema, p kitsettings.GenericProfile) error {
	sch, ok := schemas[p.Provider]
	if !ok {
		return fmt.Errorf("unknown %s provider %q", name, p.Provider)
	}
	return sch.Validate(p.Config)
}

func describeProviders(schemas map[string]schema.Schema) map[string]*systemv1.ProviderSchema {
	providers := make(map[string]*systemv1.ProviderSchema, len(schemas))
	for name, sch := range schemas {
		providers[name] = &systemv1.ProviderSchema{Fields: fieldDescriptors(sch)}
	}
	return providers
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
	schemas := sms.Schemas()
	return connect.NewResponse(&systemv1.DescribeSmsProvidersResponse{Providers: describeProviders(schemas)}), nil
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
	return profileToProto(p, sms.Schemas()[p.Provider])
}

// profileToProto masks the config via the provider's schema — secrets never
// leave the server — then encodes it as a Struct. The masked map holds only
// strings and *_set bools, so structpb encoding cannot fail.
func profileToProto(p kitsettings.GenericProfile, sch schema.Schema) *systemv1.Profile {
	cfg, err := structpb.NewStruct(sch.Mask(p.Config))
	if err != nil {
		cfg = &structpb.Struct{}
	}
	return &systemv1.Profile{Id: p.Id, Name: p.Name, Provider: p.Provider, Config: cfg}
}

func fieldDescriptors(sch schema.Schema) []*systemv1.FieldDescriptor {
	out := make([]*systemv1.FieldDescriptor, len(sch.Fields))
	for i, f := range sch.Fields {
		out[i] = &systemv1.FieldDescriptor{
			Key:       f.Key,
			Label:     f.Label,
			Type:      string(f.Type),
			Secret:    f.Secret,
			Immutable: f.Immutable,
			Required:  f.Required,
			Options:   f.Options,
			Help:      f.Help,
			MaxLen:    int32(f.MaxLen),
			Pattern:   f.Pattern,
			Min:       int32(f.Min),
			Max:       int32(f.Max),
			Unique:    f.Unique,
		}
	}
	return out
}

func toProtoSms(cfg settings.Sms) *systemv1.SmsSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = smsProfileToProto(p)
	}
	bindings := make([]*systemv1.SmsBinding, len(sms.Purposes))
	for i, purpose := range sms.Purposes {
		bindings[i] = &systemv1.SmsBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.SmsSettings{Profiles: profiles, Bindings: bindings}
}
