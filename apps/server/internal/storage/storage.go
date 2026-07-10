// Package storage wraps file storage behind a small interface so RPC
// services and tests never touch a concrete backend directly. Storage is
// organized as named connection profiles bound to fixed application purposes
// (avatars, site assets); each profile picks a driver ("s3" for any
// S3-compatible endpoint, "local" for server-disk storage) and clients are
// built per call from the current settings, so admins can reconfigure at
// runtime without a restart.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// Storage purposes are code, not data: each is a fixed slot the application
// reads/writes, and operators bind each one to a connection profile. Adding a
// feature that stores objects = adding a purpose here.
const (
	// PurposeAvatars holds user avatars, rendered on public-facing pages
	// (user lists, comments); reads are public.
	PurposeAvatars = "avatars"
	// PurposeSiteAssets holds site branding (logo, favicon) referenced from
	// public pages (including the login page, before any session); reads are
	// public.
	PurposeSiteAssets = "site-assets"
)

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{
	{Key: PurposeAvatars, Name: "用户头像", Description: "用户资料与列表展示的公开头像", Cardinality: integration.Single},
	{Key: PurposeSiteAssets, Name: "站点资源", Description: "登录页与站点导航使用的公开品牌资源", Cardinality: integration.Single},
}

// Visibility is a static property of a purpose (public / private), fixed in
// code — never stored on file rows nor editable by admins. Public means reads
// skip auth and URLs are stable and long-cacheable; private means every read
// is authenticated and served via a short-lived signed URL. Writes (PUT)
// always require credentials regardless of visibility. Drivers execute
// visibility; this table defines it.
var visibilityByPurpose = map[string]Visibility{
	PurposeAvatars:    VisibilityPublic,
	PurposeSiteAssets: VisibilityPublic,
}

type Visibility int

const (
	VisibilityPrivate Visibility = iota
	VisibilityPublic
)

// VisibilityOf returns the purpose's visibility; unknown purposes are private
// (fail closed).
func VisibilityOf(purpose string) Visibility {
	return visibilityByPurpose[purpose]
}

func (c *Client) VisibilityOf(purpose string) storageint.Visibility {
	if VisibilityOf(purpose) == VisibilityPublic {
		return storageint.VisibilityPublic
	}
	return storageint.VisibilityPrivate
}

// ErrNotConfigured signals that the purpose is unbound or its profile is
// incomplete; callers map it to a friendly "storage not configured" RPC error.
var ErrNotConfigured = fmt.Errorf("file storage is not configured")

// ObjectStore is the app-facing surface: write via presigned PUT, reclaim via
// Delete. Reads happen through the permanent /f/{file_id} handler (ADR-0004),
// never through URLs minted by RPC services.
type ObjectStore interface {
	PresignPut(ctx context.Context, purpose, key, contentType string, expires time.Duration) (string, error)
	// Delete removes an object. It is idempotent: deleting a key that no longer
	// exists returns nil, so the unattached-file sweep can safely re-run after a
	// crash (ADR-0003).
	Delete(ctx context.Context, purpose, key string) error
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, cfg kitsettings.GenericProfile) error
}

type Client struct {
	store    *settings.Store
	registry storageint.Registry
}

func NewClient(store *settings.Store, registry storageint.Registry) *Client {
	return &Client{store: store, registry: registry}
}

func (c *Client) LocalSignedURL(ctx context.Context, method, purpose, key string, expires time.Duration) (string, error) {
	secret, err := c.store.StorageSignKey(ctx)
	if err != nil {
		return "", err
	}
	return storageint.SignedURL(secret, method, purpose, key, expires), nil
}

var (
	_ ObjectStore      = (*Client)(nil)
	_ ConnectionTester = (*Client)(nil)
)

func (c *Client) PresignPut(ctx context.Context, purpose, key, contentType string, expires time.Duration) (string, error) {
	cfg, err := c.profileFor(ctx, purpose)
	if err != nil {
		return "", err
	}
	return c.registry.PresignPut(c, ctx, cfg.Provider, cfg.Config, purpose, key, contentType, expires)
}

func (c *Client) ResolveURL(ctx context.Context, purpose, key string, expires time.Duration) (string, error) {
	cfg, err := c.profileFor(ctx, purpose)
	if err != nil {
		return "", err
	}
	return c.registry.ResolveURL(c, ctx, cfg.Provider, cfg.Config, purpose, key, expires)
}

func (c *Client) Delete(ctx context.Context, purpose, key string) error {
	cfg, err := c.profileFor(ctx, purpose)
	if err != nil {
		return err
	}
	return c.registry.Delete(c, ctx, cfg.Provider, cfg.Config, purpose, key)
}

func (c *Client) TestConnection(ctx context.Context, cfg kitsettings.GenericProfile) error {
	if !c.registry.ConfigUsable(cfg.Provider, cfg.Config) {
		return ErrNotConfigured
	}
	return c.registry.Test(ctx, c, cfg.Provider, cfg.Config)
}

func (c *Client) ObjectPath(profile kitsettings.GenericProfile, key string) (string, error) {
	return c.registry.ObjectPath(profile.Provider, profile.Config, key)
}

func (c *Client) profileFor(ctx context.Context, purpose string) (kitsettings.GenericProfile, error) {
	st, err := c.store.Storage(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	cfg, ok := st.ProfileFor(purpose)
	if !ok {
		return cfg, ErrNotConfigured
	}
	if !c.registry.ConfigUsable(cfg.Provider, cfg.Config) {
		return cfg, ErrNotConfigured
	}
	return cfg, nil
}
