package rpc

import (
	"context"

	"connectrpc.com/connect"

	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

func (s *SystemService) storageOps() integrationOps[systemcodec.StorageProfile] {
	return integrationOps[systemcodec.StorageProfile]{
		name:        "storage",
		load:        s.settings.Storage,
		save:        s.settings.SetStorage,
		purposes:    storage.Purposes,
		keepSecrets: systemcodec.StorageCodec.Merge,
	}
}

func (s *SystemService) CreateStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateStorageProfileRequest],
) (*connect.Response[systemv1.CreateStorageProfileResponse], error) {
	profile, err := s.storageOps().create(ctx, s, systemcodec.StorageCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.CreateStorageProfileResponse{
		Profile: systemcodec.StorageCodec.Mask(profile),
	}), nil
}

func (s *SystemService) UpdateStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateStorageProfileRequest],
) (*connect.Response[systemv1.UpdateStorageProfileResponse], error) {
	profile, err := s.storageOps().update(ctx, s, systemcodec.StorageCodec.FromProto(req.Msg.GetProfile()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.UpdateStorageProfileResponse{
		Profile: systemcodec.StorageCodec.Mask(profile),
	}), nil
}

func (s *SystemService) DeleteStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteStorageProfileRequest],
) (*connect.Response[systemv1.DeleteStorageProfileResponse], error) {
	if err := s.storageOps().delete(ctx, s, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteStorageProfileResponse{}), nil
}

func (s *SystemService) BindStoragePurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindStoragePurposeRequest],
) (*connect.Response[systemv1.BindStoragePurposeResponse], error) {
	cfg, err := s.storageOps().bind(ctx, s, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.BindStoragePurposeResponse{
		Storage: toProtoStorage(cfg),
	}), nil
}

func (s *SystemService) TestStorageConnection(
	ctx context.Context,
	req *connect.Request[systemv1.TestStorageConnectionRequest],
) (*connect.Response[systemv1.TestStorageConnectionResponse], error) {
	var in *systemcodec.StorageProfile
	if req.Msg.GetProfile() != nil {
		p := systemcodec.StorageCodec.FromProto(req.Msg.GetProfile())
		in = &p
	}
	cfg, err := s.storageOps().resolveTestProfile(ctx, s, in, req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	if err := s.storageTester.TestConnection(ctx, cfg); err != nil {
		return connect.NewResponse(&systemv1.TestStorageConnectionResponse{
			Ok:      false,
			Message: testFailureMessage(err, storage.ErrNotConfigured, "storage is not configured: fill in the connection settings"),
		}), nil
	}
	return connect.NewResponse(&systemv1.TestStorageConnectionResponse{Ok: true}), nil
}

func toProtoStorage(cfg settings.Storage) *systemv1.StorageSettings {
	profiles := make([]*systemv1.StorageProfile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = systemcodec.StorageCodec.Mask(p)
	}
	// Bindings are emitted in catalog order so the UI renders a stable list.
	bindings := make([]*systemv1.StorageBinding, len(storage.Purposes))
	for i, purpose := range storage.Purposes {
		bindings[i] = &systemv1.StorageBinding{
			Purpose:   purpose,
			ProfileId: firstID(cfg.Bindings[purpose]),
		}
	}
	return &systemv1.StorageSettings{Profiles: profiles, Bindings: bindings}
}
