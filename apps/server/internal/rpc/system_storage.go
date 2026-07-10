package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

// systemStorage is the storage integration's admin surface: profile CRUD,
// purpose binding, connection test and provider descriptors. It owns only the
// storage tester and registry on top of the shared systemBase.
type systemStorage struct {
	systemBase
	storageTester   storage.ConnectionTester
	storageRegistry storageint.Registry
}

func (s *systemStorage) storageOps() integrationOps[kitsettings.GenericProfile] {
	return integrationOps[kitsettings.GenericProfile]{
		name:     "storage",
		load:     s.settings.Storage,
		save:     s.settings.SetStorage,
		purposes: storage.Purposes,
	}
}

func (s *systemStorage) CreateStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.CreateStorageProfileRequest],
) (*connect.Response[systemv1.CreateStorageProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Storage(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load storage settings", err)
	}
	canonical, err := s.storageRegistry.CreateConfig(
		input.GetProvider(),
		configValues(input.GetConfig()),
		input.GetConfig().GetSecrets(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	profile := kitsettings.GenericProfile{
		Id:       uuid.NewString(),
		Name:     input.GetName(),
		Provider: input.GetProvider(),
		Config:   canonical,
	}
	cfg.Profiles = append(cfg.Profiles, profile)
	if err := s.settings.SetStorage(ctx, cfg); err != nil {
		return nil, s.internal(ctx, "save storage settings", err)
	}
	return connect.NewResponse(
		&systemv1.CreateStorageProfileResponse{Profile: s.storageProfileToProto(profile)},
	), nil
}

func (s *systemStorage) UpdateStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.UpdateStorageProfileRequest],
) (*connect.Response[systemv1.UpdateStorageProfileResponse], error) {
	input := req.Msg.GetProfile()
	cfg, err := s.settings.Storage(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load storage settings", err)
	}
	for i, stored := range cfg.Profiles {
		if stored.Id != input.GetId() {
			continue
		}
		if stored.Provider != input.GetProvider() {
			return nil, connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("storage provider cannot be changed"),
			)
		}
		canonical, err := s.storageRegistry.UpdateConfig(
			stored.Provider,
			configValues(input.GetConfig()),
			input.GetConfig().GetSecrets(),
			stored.Config,
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		profile := kitsettings.GenericProfile{
			Id:       stored.Id,
			Name:     input.GetName(),
			Provider: stored.Provider,
			Config:   canonical,
		}
		cfg.Profiles[i] = profile
		if err := s.settings.SetStorage(ctx, cfg); err != nil {
			return nil, s.internal(ctx, "save storage settings", err)
		}
		return connect.NewResponse(
			&systemv1.UpdateStorageProfileResponse{Profile: s.storageProfileToProto(profile)},
		), nil
	}
	return nil, s.storageOps().errNotFound()
}

func (s *systemStorage) DeleteStorageProfile(
	ctx context.Context,
	req *connect.Request[systemv1.DeleteStorageProfileRequest],
) (*connect.Response[systemv1.DeleteStorageProfileResponse], error) {
	if err := s.storageOps().delete(ctx, &s.systemBase, req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&systemv1.DeleteStorageProfileResponse{}), nil
}

func (s *systemStorage) BindStoragePurpose(
	ctx context.Context,
	req *connect.Request[systemv1.BindStoragePurposeRequest],
) (*connect.Response[systemv1.BindStoragePurposeResponse], error) {
	cfg, err := s.storageOps().
		bind(ctx, &s.systemBase, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(
		&systemv1.BindStoragePurposeResponse{Storage: s.toProtoStorage(cfg)},
	), nil
}

func (s *systemStorage) TestStorageConnection(
	ctx context.Context,
	req *connect.Request[systemv1.TestStorageConnectionRequest],
) (*connect.Response[systemv1.TestStorageConnectionResponse], error) {
	profile, err := s.resolveStorageTestProfile(ctx, req.Msg.GetProfile(), req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}
	if err := s.storageTester.TestConnection(ctx, profile); err != nil {
		return connect.NewResponse(
			&systemv1.TestStorageConnectionResponse{
				Ok: false,
				Message: testFailureMessage(
					err,
					storage.ErrNotConfigured,
					"storage is not configured: fill in the connection settings",
				),
			},
		), nil
	}
	return connect.NewResponse(&systemv1.TestStorageConnectionResponse{Ok: true}), nil
}

func (s *systemStorage) resolveStorageTestProfile(
	ctx context.Context,
	input *systemv1.ProfileInput,
	id string,
) (kitsettings.GenericProfile, error) {
	cfg, err := s.settings.Storage(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, s.internal(ctx, "load storage settings", err)
	}
	if input != nil {
		if stored, ok := cfg.Profile(input.GetId()); ok {
			if stored.Provider != input.GetProvider() {
				return kitsettings.GenericProfile{}, connect.NewError(
					connect.CodeInvalidArgument,
					errors.New("storage provider cannot be changed"),
				)
			}
			canonical, err := s.storageRegistry.UpdateConfig(
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
				Id:       stored.Id,
				Name:     input.GetName(),
				Provider: stored.Provider,
				Config:   canonical,
			}, nil
		}
		canonical, err := s.storageRegistry.CreateConfig(
			input.GetProvider(),
			configValues(input.GetConfig()),
			input.GetConfig().GetSecrets(),
		)
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
		if stored, ok := cfg.Profile(id); ok {
			return stored, nil
		}
		return kitsettings.GenericProfile{}, s.storageOps().errNotFound()
	}
	return kitsettings.GenericProfile{}, connect.NewError(
		connect.CodeInvalidArgument,
		errors.New("provide a profile or profile_id to test"),
	)
}

func (s *systemStorage) storageSnapshot(ctx context.Context) (*systemv1.StorageSettings, error) {
	cfg, err := s.settings.Storage(ctx)
	if err != nil {
		return nil, s.internal(ctx, "load storage settings", err)
	}
	return s.toProtoStorage(cfg), nil
}

func (s *systemStorage) toProtoStorage(cfg settings.Storage) *systemv1.StorageSettings {
	profiles := make([]*systemv1.Profile, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		profiles[i] = s.storageProfileToProto(p)
	}
	bindings := make([]*systemv1.StorageBinding, len(storage.Purposes))
	for i, purpose := range storage.Purposes {
		bindings[i] = &systemv1.StorageBinding{
			Purpose:   purpose.Key,
			ProfileId: firstID(cfg.Bindings[purpose.Key]),
		}
	}
	return &systemv1.StorageSettings{Profiles: profiles, Bindings: bindings}
}

func (s *systemStorage) DescribeStorageProviders(
	_ context.Context,
	_ *connect.Request[systemv1.DescribeStorageProvidersRequest],
) (*connect.Response[systemv1.DescribeStorageProvidersResponse], error) {
	return connect.NewResponse(
		&systemv1.DescribeStorageProvidersResponse{
			Purposes:  describePurposes(storage.Purposes),
			Providers: describeStorageProviders(s.storageRegistry),
		},
	), nil
}
func describeStorageProviders(registry storageint.Registry) []*systemv1.ProviderDescriptor {
	descriptors := registry.Descriptors()
	out := make([]*systemv1.ProviderDescriptor, len(descriptors))
	for i, d := range descriptors {
		out[i] = &systemv1.ProviderDescriptor{
			Key:          d.Key,
			Presentation: presentationToProto(d.Presentation),
			Config: &systemv1.ProviderForm{
				Schema:   toStruct(d.JSONSchema),
				UiSchema: toStruct(d.UISchema),
			},
		}
	}
	return out
}
func (s *systemStorage) storageProfileToProto(p kitsettings.GenericProfile) *systemv1.Profile {
	view, valid := s.storageRegistry.ViewConfig(p.Provider, p.Config)
	return &systemv1.Profile{
		Id:       p.Id,
		Name:     p.Name,
		Provider: p.Provider,
		Config: &systemv1.ConfigView{
			Values:         toStruct(view.Values),
			SetSecretPaths: view.SetSecretPaths,
		},
		ConfigValid: valid,
	}
}
