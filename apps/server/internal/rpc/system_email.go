package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	emailint "github.com/imbytecat/moonbase/integrations/email"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	mail "github.com/imbytecat/moonbase/server/internal/mail"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// systemEmail is the email integration's admin surface: profile CRUD, purpose
// binding, test send and provider descriptors. It owns only the email registry
// and mailer on top of the shared systemBase.
type systemEmail struct {
	systemBase
	emailRegistry emailint.Registry
	mailer        mail.ProfileSender
}

func (s *systemEmail) emailOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "email",
		load:     s.settings.Email,
		save:     s.settings.SetEmail,
		purposes: mail.Purposes,
	}
}

func (s *systemEmail) CreateEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateEmailProfileRequest],
) (*connect.Response[systemv1.CreateEmailProfileResponse], error) {
	input := req.Msg.GetProfile()
	settings, err := s.settings.Email(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load email settings", err)
	}
	canonical, err := s.emailRegistry.CreateConfig(
		input.GetProvider(), configValues(input.GetConfig()), input.GetConfig().GetSecrets())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	profile := kitsettings.GenericProfile{
		Id:       uuid.NewString(),
		Name:     input.GetName(),
		Provider: input.GetProvider(),
		Config:   canonical,
	}
	settings.Profiles = append(settings.Profiles, profile)
	if err := s.settings.SetEmail(ctx, settings); err != nil {
		return nil, s.internal(ctx, "save email settings", err)
	}
	return connect.NewResponse(&systemv1.CreateEmailProfileResponse{
		Profile: s.emailProfileToProto(profile),
	}), nil
}

func (s *systemEmail) UpdateEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateEmailProfileRequest],
) (*connect.Response[systemv1.UpdateEmailProfileResponse], error) {
	input := req.Msg.GetProfile()
	settings, err := s.settings.Email(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load email settings", err)
	}
	for i, stored := range settings.Profiles {
		if stored.Id != input.GetId() {
			continue
		}
		if stored.Provider != input.GetProvider() {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("email provider cannot be changed"))
		}
		canonical, err := s.emailRegistry.UpdateConfig(
			stored.Provider,
			configValues(input.GetConfig()),
			input.GetConfig().GetSecrets(),
			stored.Config,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		updated := kitsettings.GenericProfile{
			Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical,
		}
		settings.Profiles[i] = updated
		if err := s.settings.SetEmail(ctx, settings); err != nil {
			return nil, s.internal(ctx, "save email settings", err)
		}
		return connect.NewResponse(&systemv1.UpdateEmailProfileResponse{
			Profile: s.emailProfileToProto(updated),
		}), nil
	}
	return nil, s.emailOps().errNotFound()
}

func (s *systemEmail) DeleteEmailProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteEmailProfileRequest],
) (*connect.Response[systemv1.DeleteEmailProfileResponse], error) {
	if err := s.emailOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteEmailProfileResponse{}), nil
}

func (s *systemEmail) BindEmailPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindEmailPurposeRequest],
) (*connect.Response[systemv1.BindEmailPurposeResponse], error) {
	cfg, err := s.emailOps().bind(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindEmailPurposeResponse{
		Email: s.toProtoEmail(cfg),
	}), nil
}

func (s *systemEmail) SendTestEmail(
	ctx context.Context,
	req *connect.Request[systemv1.SendTestEmailRequest],
) (*connect.Response[systemv1.SendTestEmailResponse], error) {
	profile, err := s.resolveEmailTestProfile(ctx, req.Msg.GetProfile(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	err = s.mailer.SendWith(ctx, profile, req.Msg.GetTo(),
		"Test email", "This is a test email from your admin panel. Delivery is working.")
	if err != nil {
		return connect.NewResponse(&systemv1.SendTestEmailResponse{
			Ok: false,
			Message: testFailureMessage(
				err,
				mail.ErrNotConfigured,
				"email is not configured: fill in the delivery settings and from address",
			),
		}), nil
	}
	return connect.NewResponse(&systemv1.SendTestEmailResponse{Ok: true}), nil
}

func (s *systemEmail) resolveEmailTestProfile(
	ctx context.Context,
	input *systemv1.ProfileInput,
	id string,
) (kitsettings.GenericProfile, error) {
	settings, err := s.settings.Email(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, s.internal(ctx, "load email settings", err)
	}
	if input != nil {
		if stored, ok := settings.Profile(input.GetId()); ok {
			if stored.Provider != input.GetProvider() {
				return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument,
					errors.New("email provider cannot be changed"))
			}
			canonical, err := s.emailRegistry.UpdateConfig(
				stored.Provider,
				configValues(input.GetConfig()),
				input.GetConfig().GetSecrets(),
				stored.Config,
			)
			if err != nil {
				return kitsettings.GenericProfile{}, connect.NewError(
					connect.CodeInvalidArgument,
					err,
				)
			}
			return kitsettings.GenericProfile{
				Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical,
			}, nil
		}
		canonical, err := s.emailRegistry.CreateConfig(
			input.GetProvider(), configValues(input.GetConfig()), input.GetConfig().GetSecrets())
		if err != nil {
			return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return kitsettings.GenericProfile{
			Id:       input.GetId(),
			Name:     input.GetName(),
			Provider: input.GetProvider(),
			Config:   canonical,
		}, nil
	}
	if id != "" {
		if stored, ok := settings.Profile(id); ok {
			return stored, nil
		}
		return kitsettings.GenericProfile{}, s.emailOps().errNotFound()
	}
	return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument,
		errors.New("provide a profile or profile_id to test"))
}

func configValues(write *systemv1.ConfigWrite) map[string]any {
	if write == nil || write.GetValues() == nil {
		return map[string]any{}
	}
	return write.GetValues().AsMap()
}

func (s *systemEmail) emailSnapshot(ctx context.Context) (*systemv1.EmailSettings, error) {
	cfg, err := s.settings.Email(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load email settings", err)
	}
	return s.toProtoEmail(cfg), nil
}

func (s *systemEmail) toProtoEmail(cfg settings.Email) *systemv1.EmailSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.emailProfileToProto(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.EmailBinding, len(mail.Purposes))
	for i, purpose := range mail.Purposes {
		bindings[i] = &systemv1.EmailBinding{
			Purpose:   purpose.Key,
			ProfileId: firstID(cfg.Bindings[purpose.Key]),
		}
	}
	return &systemv1.EmailSettings{Profiles: profiles, Bindings: bindings}
}

func (s *systemEmail) DescribeEmailProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeEmailProvidersRequest],
) (*connect.Response[systemv1.DescribeEmailProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeEmailProvidersResponse{
		Purposes: describePurposes(
			mail.Purposes,
		),
		Providers: describeEmailProviders(s.emailRegistry),
	}), nil
}

func describeEmailProviders(registry emailint.Registry) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, descriptor := range descriptors {
		out[i] = &systemv1.ProviderDescriptor{
			Key:          descriptor.Key,
			Presentation: presentationToProto(descriptor.Presentation),
			Config: &systemv1.ProviderForm{
				Schema: toStruct(descriptor.JSONSchema), UiSchema: toStruct(descriptor.UISchema),
			},
		}
	}
	return out
}

func (s *systemEmail) emailProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.emailRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{
		Id: p.Id, Name: p.Name, Provider: p.Provider,
		Config: &systemv1.ConfigView{
			Values: toStruct(view.Values), SetSecretPaths: view.SetSecretPaths,
		},
		ConfigValid: valid,
	}
}
