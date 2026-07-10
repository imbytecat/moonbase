package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/llm"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

func (s *SystemService) llmOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{name: "model", load: s.settings.Llm, save: s.settings.SetLlm, purposes: llm.Purposes}
}
func (s *SystemService) CreateLlmProfile(ctx context.Context, req *connect.Request[systemv1.CreateLlmProfileRequest]) (*connect.Response[systemv1.CreateLlmProfileResponse], error) {
	in := req.Msg.GetProfile()
	cfg, err := s.settings.Llm(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load llm settings", err)
	}
	canonical, err := s.llmRegistry.CreateConfig(in.GetProvider(), configValues(in.GetConfig()), in.GetConfig().GetSecrets())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	p := kitsettings.GenericProfile{Id: uuid.NewString(), Name: in.GetName(), Provider: in.GetProvider(), Config: canonical}
	cfg.Profiles = append(cfg.Profiles, p)
	if err := s.settings.SetLlm(ctx, cfg); err != nil {
		return nil, s.internal(ctx, "save llm settings", err)
	}
	return connect.NewResponse(&systemv1.CreateLlmProfileResponse{Profile: s.llmProfileToProto(p)}), nil
}
func (s *SystemService) UpdateLlmProfile(ctx context.Context, req *connect.Request[systemv1.UpdateLlmProfileRequest]) (*connect.Response[systemv1.UpdateLlmProfileResponse], error) {
	in := req.Msg.GetProfile()
	cfg, err := s.settings.Llm(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load llm settings", err)
	}
	for i, old := range cfg.Profiles {
		if old.Id != in.GetId() {
			continue
		}
		if old.Provider != in.GetProvider() {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("llm provider cannot be changed"))
		}
		canonical, err := s.llmRegistry.UpdateConfig(old.Provider, configValues(in.GetConfig()), in.GetConfig().GetSecrets(), old.Config)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		p := kitsettings.GenericProfile{Id: old.Id, Name: in.GetName(), Provider: old.Provider, Config: canonical}
		cfg.Profiles[i] = p
		if err := s.settings.SetLlm(ctx, cfg); err != nil {
			return nil, s.internal(ctx, "save llm settings", err)
		}
		return connect.NewResponse(&systemv1.UpdateLlmProfileResponse{Profile: s.llmProfileToProto(p)}), nil
	}
	return nil, s.llmOps().errNotFound()
}
func (s *SystemService) DeleteLlmProfile(ctx context.Context, req *connect.Request[systemv1.DeleteLlmProfileRequest]) (*connect.Response[systemv1.DeleteLlmProfileResponse], error) {
	if err := s.llmOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteLlmProfileResponse{}), nil
}
func (s *SystemService) BindLlmPurpose(ctx context.Context, req *connect.Request[systemv1.BindLlmPurposeRequest]) (*connect.Response[systemv1.BindLlmPurposeResponse], error) {
	cfg, err := s.llmOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindLlmPurposeResponse{Llm: s.toProtoLlm(cfg)}), nil
}
func (s *SystemService) TestLlm(ctx context.Context, req *connect.Request[systemv1.TestLlmRequest]) (*connect.Response[systemv1.TestLlmResponse], error) {
	p, err := s.resolveLlmTestProfile(ctx, req.Msg.GetProfile(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	reply, err := s.chatter.CompleteWith(ctx, p, "You are a connectivity check. Reply with a single short sentence.", "Say hello and name the model you are.")
	if err != nil {
		return connect.NewResponse(&systemv1.TestLlmResponse{Ok: false, Message: testFailureMessage(err, llm.ErrNotConfigured, "ai model is not configured: fill in an API key and model")}), nil
	}
	return connect.NewResponse(&systemv1.TestLlmResponse{Ok: true, Message: reply}), nil
}
func (s *SystemService) resolveLlmTestProfile(ctx context.Context, in *systemv1.ProfileInput, id string) (kitsettings.GenericProfile, error) {
	cfg, err := s.settings.Llm(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, s.internal(ctx, "load llm settings", err)
	}
	if in != nil {
		if old, ok := cfg.Profile(in.GetId()); ok {
			if old.Provider != in.GetProvider() {
				return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, errors.New("llm provider cannot be changed"))
			}
			canonical, err := s.llmRegistry.UpdateConfig(old.Provider, configValues(in.GetConfig()), in.GetConfig().GetSecrets(), old.Config)
			if err != nil {
				return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, err)
			}
			return kitsettings.GenericProfile{Id: old.Id, Name: in.GetName(), Provider: old.Provider, Config: canonical}, nil
		}
		canonical, err := s.llmRegistry.CreateConfig(in.GetProvider(), configValues(in.GetConfig()), in.GetConfig().GetSecrets())
		if err != nil {
			return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return kitsettings.GenericProfile{Name: in.GetName(), Provider: in.GetProvider(), Config: canonical}, nil
	}
	if id != "" {
		if p, ok := cfg.Profile(id); ok {
			return p, nil
		}
		return kitsettings.GenericProfile{}, s.llmOps().errNotFound()
	}
	return kitsettings.GenericProfile{}, connect.NewError(connect.CodeInvalidArgument, errors.New("provide a profile or profile_id to test"))
}
func (s *SystemService) toProtoLlm(cfg settings.Llm) *systemv1.LlmSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.llmProfileToProto(p)
	}
	bindings := make([]*systemv1.LlmBinding, len(llm.Purposes))
	for i, p := range llm.Purposes {
		bindings[i] = &systemv1.LlmBinding{Purpose: p.Key, ProfileId: firstID(cfg.Bindings[p.Key])}
	}
	return &systemv1.LlmSettings{Profiles: profiles, Bindings: bindings}
}
func (s *SystemService) DescribeLlmProviders(_ context.Context, _ *connect.Request[systemv1.DescribeLlmProvidersRequest]) (*connect.Response[systemv1.DescribeLlmProvidersResponse], error) {
	return connect.NewResponse(&systemv1.DescribeLlmProvidersResponse{Purposes: describePurposes(llm.Purposes), Providers: describeLlmProviders(s.llmRegistry)}), nil
}
func describeLlmProviders(r llmint.Registry) []*systemv1.ProviderDescriptor {
	ds := r.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(ds))
	for i, d := range ds {
		out[i] = &systemv1.ProviderDescriptor{Key: d.Key, Presentation: presentationToProto(d.Presentation), Config: &systemv1.ProviderForm{Schema: toStruct(d.JSONSchema), UiSchema: toStruct(d.UISchema)}}
	}
	return out
}
func (s *SystemService) llmProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.llmRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{Id: p.Id, Name: p.Name, Provider: p.Provider, Config: &systemv1.ConfigView{Values: toStruct(view.Values), SetSecretPaths: view.SetSecretPaths}, ConfigValid: valid}
}
