package rpc

import (
	"context"

	"connectrpc.com/connect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/llm"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

func (s *SystemService) llmOps() integrationOps[systemcodec.LlmProfile] {
	return integrationOps[systemcodec.LlmProfile]{
		name:        "model",
		load:        s.settings.Llm,
		save:        s.settings.SetLlm,
		purposes:    llm.Purposes,
		keepSecrets: systemcodec.LlmCodec.Merge,
	}
}

func (s *SystemService) CreateLlmProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateLlmProfileRequest],
) (*connect.Response[systemv1.CreateLlmProfileResponse], error) {
	profile, err := s.llmOps().create(ctx, s, systemcodec.LlmCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateLlmProfileResponse{
		Profile: systemcodec.LlmCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdateLlmProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateLlmProfileRequest],
) (*connect.Response[systemv1.UpdateLlmProfileResponse], error) {
	profile, err := s.llmOps().update(ctx, s, systemcodec.LlmCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateLlmProfileResponse{
		Profile: systemcodec.LlmCodec.Mask(profile),
	}), nil
}

func (s *SystemService) DeleteLlmProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteLlmProfileRequest],
) (*connect.Response[systemv1.DeleteLlmProfileResponse], error) {
	if err := s.llmOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteLlmProfileResponse{}), nil
}

func (s *SystemService) BindLlmPurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindLlmPurposeRequest],
) (*connect.Response[systemv1.BindLlmPurposeResponse], error) {
	cfg, err := s.llmOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindLlmPurposeResponse{
		Llm: toProtoLlm(cfg),
	}), nil
}

func (s *SystemService) TestLlm(
	ctx context.Context,
	req *connect.Request[systemv1.TestLlmRequest],
) (*connect.Response[systemv1.TestLlmResponse], error) {
	var in *systemcodec.LlmProfile
	if req.Msg.GetProfile() != nil {
		p := systemcodec.LlmCodec.FromProto(req.Msg.GetProfile())
		in = &p
	}
	profile, err := s.llmOps().resolveTestProfile(ctx, s, in, req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	reply, err := s.chatter.CompleteWith(ctx, profile,
		"You are a connectivity check. Reply with a single short sentence.",
		"Say hello and name the model you are.")
	if err != nil {
		return connect.NewResponse(&systemv1.TestLlmResponse{
			Ok:      false,
			Message: testFailureMessage(err, llm.ErrNotConfigured, "ai model is not configured: fill in an API key and model"),
		}), nil
	}
	return connect.NewResponse(&systemv1.TestLlmResponse{
		Ok:      true,
		Message: reply,
	}), nil
}

func toProtoLlm(cfg settings.Llm) *systemv1.LlmSettings {
	profiles := make([]*systemv1.LlmProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.LlmCodec.Mask(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.LlmBinding, len(llm.Purposes))
	for i, purpose := range llm.Purposes {
		bindings[i] = &systemv1.LlmBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.LlmSettings{Profiles: profiles, Bindings: bindings}
}
