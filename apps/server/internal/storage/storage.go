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

	"github.com/imbytecat/moonbase/packages/integrations/core/integration"
	"github.com/imbytecat/moonbase/packages/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
	storageint "github.com/imbytecat/moonbase/packages/integrations/storage"
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
var Purposes = integration.Catalog{PurposeAvatars, PurposeSiteAssets}

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

// storageOps is the per-provider seam: each backend implements the three
// storage verbs against its own schema-described config shape. purpose
// rides along because the local driver embeds it in signed URLs (the HTTP
// handler re-resolves purpose → profile → directory at request time, so
// rebinding a purpose never leaves stale URLs pointing at the wrong
// directory).
func Schemas() map[string]schema.Schema { return storageint.Schemas() }

// Providers lists the registered driver names, sorted.
func Providers() []string {
	return storageint.Providers()
}

func ProfileUsable(p kitsettings.GenericProfile) bool {
	return storageint.ProfileUsable(p)
}

type Client struct {
	store *settings.Store
}

func NewClient(store *settings.Store) *Client {
	return &Client{store: store}
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
	ops, cfg, err := c.opsFor(ctx, purpose)
	if err != nil {
		return "", err
	}
	return ops.PresignPut(c, ctx, cfg, purpose, key, contentType, expires)
}

func (c *Client) ResolveURL(ctx context.Context, purpose, key string, expires time.Duration) (string, error) {
	ops, cfg, err := c.opsFor(ctx, purpose)
	if err != nil {
		return "", err
	}
	return ops.ResolveURL(c, ctx, cfg, purpose, key, expires)
}

func (c *Client) Delete(ctx context.Context, purpose, key string) error {
	ops, cfg, err := c.opsFor(ctx, purpose)
	if err != nil {
		return err
	}
	return ops.Delete(c, ctx, cfg, purpose, key)
}

func (c *Client) TestConnection(ctx context.Context, cfg kitsettings.GenericProfile) error {
	d, ok := storageint.DriverFor(cfg.Provider)
	if !ok || !d.Schema.Usable(cfg.Config) {
		return ErrNotConfigured
	}
	return d.Ops.Test(c, ctx, cfg)
}

func (c *Client) opsFor(ctx context.Context, purpose string) (storageint.Ops, kitsettings.GenericProfile, error) {
	st, err := c.store.Storage(ctx)
	if err != nil {
		return storageint.Ops{}, kitsettings.GenericProfile{}, err
	}
	cfg, ok := st.ProfileFor(purpose)
	if !ok {
		return storageint.Ops{}, cfg, ErrNotConfigured
	}
	d, ok := storageint.DriverFor(cfg.Provider)
	if !ok || !d.Schema.Usable(cfg.Config) {
		return storageint.Ops{}, cfg, ErrNotConfigured
	}
	return d.Ops, cfg, nil
}
