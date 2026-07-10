package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/oauth"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// systemOauth is the login-provider integration's admin surface: profile CRUD,
// login binding and provider descriptors. Beyond the shared systemBase it owns
// the oauth registry and the repository (it guards deletes against live
// identities via CountIdentitiesByProvider).
type systemOauth struct {
	systemBase
	oauthRegistry oauthint.Registry
	repo          repository.Querier
}

func (s *systemOauth) oauthOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "login provider",
		load:     s.settings.Oauth,
		save:     s.settings.SetOauth,
		purposes: oauth.Purposes,
	}
}

func (s *systemOauth) CreateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateOauthProfileRequest],
) (*connect.Response[systemv1.CreateOauthProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	values := configValues(input.GetConfig())
	key := configString(values, "key")
	if _, exists := settings.ProfileByKey(cfg, key); exists {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a login provider with key %q already exists", key))
	}
	canonical, err := s.oauthRegistry.CreateConfig(input.GetProvider(), values, input.GetConfig().GetSecrets())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	profile := kitsettings.GenericProfile{Id: uuid.NewString(), Name: input.GetName(), Provider: input.GetProvider(), Config: canonical}
	cfg.Profiles = append(cfg.Profiles, profile)
	if err := s.settings.SetOauth(ctx, cfg); err != nil {
		return nil, s.internal(ctx, "save oauth settings", err)
	}
	return connect.NewResponse(&systemv1.CreateOauthProfileResponse{
		Profile: s.oauthProfileToProto(profile),
	}), nil
}

func (s *systemOauth) UpdateOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateOauthProfileRequest],
) (*connect.Response[systemv1.UpdateOauthProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	for i, stored := range cfg.Profiles {
		if stored.Id != input.GetId() {
			continue
		}
		if stored.Provider != input.GetProvider() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("oauth provider cannot be changed"))
		}
		canonical, err := s.oauthRegistry.UpdateConfig(stored.Provider, configValues(input.GetConfig()), input.GetConfig().GetSecrets(), stored.Config)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		key := configString(canonical, "key")
		if existing, exists := settings.ProfileByKey(cfg, key); exists && existing.Id != stored.Id {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("a login provider with key %q already exists", key))
		}
		updated := kitsettings.GenericProfile{Id: stored.Id, Name: input.GetName(), Provider: stored.Provider, Config: canonical}
		cfg.Profiles[i] = updated
		if err := s.settings.SetOauth(ctx, cfg); err != nil {
			return nil, s.internal(ctx, "save oauth settings", err)
		}
		return connect.NewResponse(&systemv1.UpdateOauthProfileResponse{Profile: s.oauthProfileToProto(updated)}), nil
	}
	return nil, s.oauthOps().errNotFound()
}

func (s *systemOauth) BindOauthPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindOauthPurposeRequest],
) (*connect.Response[systemv1.BindOauthPurposeResponse], error) {
	cfg, err := s.oauthOps().bindMany(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileIds())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindOauthPurposeResponse{
		Oauth: s.toProtoOauth(cfg),
	}), nil
}

func (s *systemOauth) DeleteOauthProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteOauthProfileRequest],
) (*connect.Response[systemv1.DeleteOauthProfileResponse], error) {
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	profile, ok := cfg.Profile(req.Msg.GetId())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("login provider not found"))
	}
	// Bound identities would become unreachable orphans; unbinding the
	// login purpose is the reversible way to retire a provider.
	view, _ := s.oauthRegistry.ViewConfig(profile.Provider, profile.Config)
	count, err := s.repo.CountIdentitiesByProvider(ctx, configString(view.Values, "key"))
	if err != nil {
		return nil, s.internal(ctx, "count identities", err)
	}
	if count > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("%d account(s) still sign in through this provider — unbind it from the sign-in page instead", count))
	}
	if err := s.oauthOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteOauthProfileResponse{}), nil
}

func (s *systemOauth) oauthSnapshot(ctx context.Context) (*systemv1.OauthSettings, error) {
	cfg, err := s.settings.Oauth(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load oauth settings", err)
	}
	return s.toProtoOauth(cfg), nil
}

func (s *systemOauth) toProtoOauth(cfg settings.OAuth) *systemv1.OauthSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.oauthProfileToProto(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.OauthBinding, len(oauth.Purposes))
	for i, purpose := range oauth.Purposes {
		bindings[i] = &systemv1.OauthBinding{
			Purpose:    purpose.Key,
			ProfileIds: cfg.Bindings[purpose.Key],
		}
	}
	return &systemv1.OauthSettings{Profiles: profiles, Bindings: bindings}
}

func (s *systemOauth) DescribeOauthProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeOauthProvidersRequest],
) (*connect.Response[systemv1.DescribeOauthProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeOauthProvidersResponse{
		Purposes: describePurposes(oauth.Purposes), Providers: describeOauthProviders(s.oauthRegistry),
	}), nil
}

func describeOauthProviders(registry oauthint.Registry) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, descriptor := range descriptors {
		out[i] = &systemv1.ProviderDescriptor{Key: descriptor.Key, Presentation: presentationToProto(descriptor.Presentation), Config: &systemv1.ProviderForm{Schema: toStruct(descriptor.JSONSchema), UiSchema: toStruct(descriptor.UISchema)}}
	}
	return out
}

func (s *systemOauth) oauthProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.oauthRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{Id: p.Id, Name: p.Name, Provider: p.Provider, Config: &systemv1.ConfigView{Values: toStruct(view.Values), SetSecretPaths: view.SetSecretPaths}, ConfigValid: valid}
}

func configString(values map[string]any, key string) string {
	s, _ := values[key].(string)
	return s
}
