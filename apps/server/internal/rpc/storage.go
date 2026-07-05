package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"github.com/imbytecat/moonbase/server/internal/auth"
	storagev1 "github.com/imbytecat/moonbase/server/internal/gen/storage/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/storage/v1/storagev1connect"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/storage"
)

const uploadURLTTL = 15 * time.Minute

var contentTypeExt = map[string]string{
	"image/jpeg":               "jpg",
	"image/png":                "png",
	"image/webp":               "webp",
	"image/svg+xml":            "svg",
	"image/x-icon":             "ico",
	"image/vnd.microsoft.icon": "ico",
}

type StorageService struct {
	repo    repository.Querier
	objects storage.ObjectStore
	logger  *slog.Logger
}

func NewStorageService(repo repository.Querier, objects storage.ObjectStore, logger *slog.Logger) *StorageService {
	return &StorageService{repo: repo, objects: objects, logger: logger}
}

var _ storagev1connect.StorageServiceHandler = (*StorageService)(nil)

func (s *StorageService) PresignAvatarUpload(
	ctx context.Context,
	req *connect.Request[storagev1.PresignAvatarUploadRequest],
) (*connect.Response[storagev1.PresignAvatarUploadResponse], error) {
	id := auth.IdentityFromContext(ctx)

	// The server picks the object key — callers can never write outside their
	// own avatar prefix, and a random suffix defeats stale CDN/browser caches.
	key := fmt.Sprintf("avatars/%s/%s.%s",
		id.UserID, randomSuffix(), contentTypeExt[req.Msg.GetContentType()])

	url, err := s.objects.PresignPut(ctx, storage.PurposeAvatars, key,
		req.Msg.GetContentType(), uploadURLTTL)
	if err != nil {
		return nil, s.presignError(ctx, err)
	}
	file, err := s.repo.InsertFile(ctx, repository.InsertFileParams{
		ObjectKey:   key,
		ContentType: req.Msg.GetContentType(),
		UploadedBy:  id.UserID,
	})
	if err != nil {
		return nil, s.internal(ctx, "record avatar file", err)
	}
	return connect.NewResponse(&storagev1.PresignAvatarUploadResponse{
		UploadUrl: url,
		ObjectKey: key,
		FileId:    file.ID.String(),
	}), nil
}

func (s *StorageService) PresignSiteAssetUpload(
	ctx context.Context,
	req *connect.Request[storagev1.PresignSiteAssetUploadRequest],
) (*connect.Response[storagev1.PresignSiteAssetUploadResponse], error) {
	id := auth.IdentityFromContext(ctx)

	// kind is validated by protovalidate ("logo" | "favicon"); the random
	// suffix busts caches when branding is replaced.
	key := fmt.Sprintf("site/%s-%s.%s",
		req.Msg.GetKind(), randomSuffix(), contentTypeExt[req.Msg.GetContentType()])

	url, err := s.objects.PresignPut(ctx, storage.PurposeSiteAssets, key,
		req.Msg.GetContentType(), uploadURLTTL)
	if err != nil {
		return nil, s.presignError(ctx, err)
	}
	file, err := s.repo.InsertFile(ctx, repository.InsertFileParams{
		ObjectKey:   key,
		ContentType: req.Msg.GetContentType(),
		UploadedBy:  id.UserID,
	})
	if err != nil {
		return nil, s.internal(ctx, "record site asset file", err)
	}
	return connect.NewResponse(&storagev1.PresignSiteAssetUploadResponse{
		UploadUrl: url,
		ObjectKey: key,
		FileId:    file.ID.String(),
	}), nil
}

func (s *StorageService) presignError(ctx context.Context, err error) error {
	if errors.Is(err, storage.ErrNotConfigured) {
		return connect.NewError(connect.CodeFailedPrecondition,
			errors.New("file storage is not configured"))
	}
	return s.internal(ctx, "presign upload", err)
}

func randomSuffix() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(buf)
}

func (s *StorageService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
