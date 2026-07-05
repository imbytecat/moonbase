package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imbytecat/moonbase/server/internal/repository"
)

// purposeSiteAssets mirrors storage.PurposeSiteAssets; the storage package
// imports settings, so referencing it here would cycle. This literal is the
// same stable contract string presign, bindings and the migration all use.
const purposeSiteAssets = "site-assets"

// legacySite reads a site setting that may still hold the pre-ledger raw object
// keys (logoKey/faviconKey) alongside the new file-id fields, so the backfill
// can tell which slots still need migrating.
type legacySite struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	LogoKey       string `json:"logoKey"`
	FaviconKey    string `json:"faviconKey"`
	LogoFileID    string `json:"logoFileId"`
	FaviconFileID string `json:"faviconFileId"`
	Copyright     string `json:"copyright"`
	IcpBeian      string `json:"icpBeian"`
}

// BackfillSiteAssets migrates existing site branding off raw object keys onto
// file-ledger references (ADR-0003 存量回填). For each brand slot still holding
// a raw key it mints a files row plus an attachment and rewrites the setting to
// reference it by id, all in one transaction. It is idempotent: once a slot has
// a file id (and the raw key is gone), re-runs are no-ops, so it is safe to run
// on every startup.
func BackfillSiteAssets(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin site backfill: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := repository.New(tx)
	row, err := q.GetSetting(ctx, keySite)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read site settings: %w", err)
	}

	var legacy legacySite
	if err := json.Unmarshal(row.Value, &legacy); err != nil {
		return fmt.Errorf("decode site settings: %w", err)
	}

	changed := false
	if legacy.LogoKey != "" && legacy.LogoFileID == "" {
		id, err := mintSiteAssetFile(ctx, q, legacy.LogoKey)
		if err != nil {
			return err
		}
		legacy.LogoFileID = id
		changed = true
	}
	if legacy.FaviconKey != "" && legacy.FaviconFileID == "" {
		id, err := mintSiteAssetFile(ctx, q, legacy.FaviconKey)
		if err != nil {
			return err
		}
		legacy.FaviconFileID = id
		changed = true
	}
	if !changed {
		return nil
	}

	// SetSite drops the raw keys (the Site struct has no fields for them) and
	// creates the attachments via its atomic CTE.
	if err := NewStore(q).SetSite(ctx, Site{
		Name:          legacy.Name,
		Description:   legacy.Description,
		LogoFileID:    legacy.LogoFileID,
		FaviconFileID: legacy.FaviconFileID,
		Copyright:     legacy.Copyright,
		IcpBeian:      legacy.IcpBeian,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit site backfill: %w", err)
	}
	logger.InfoContext(ctx, "backfilled site branding into the file ledger",
		"logo", legacy.LogoFileID != "", "favicon", legacy.FaviconFileID != "")
	return nil
}

func mintSiteAssetFile(ctx context.Context, q repository.Querier, key string) (string, error) {
	file, err := q.InsertFile(ctx, repository.InsertFileParams{
		ObjectKey:   key,
		ContentType: inferSiteAssetContentType(key),
		UploadedBy:  uuid.Nil,
		Purpose:     purposeSiteAssets,
	})
	if err != nil {
		return "", fmt.Errorf("mint site asset file: %w", err)
	}
	return file.ID.String(), nil
}

func inferSiteAssetContentType(key string) string {
	switch {
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".jpg"), strings.HasSuffix(key, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(key, ".ico"):
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
