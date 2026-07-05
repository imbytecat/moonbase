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

	"github.com/imbytecat/moonbase/server/internal/channel"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// Storage purposes are code, not data: each is a fixed slot the application
// reads/writes, and operators bind each one to a connection profile. Adding a
// feature that stores objects = adding a purpose here.
const (
	// PurposeAvatars holds user avatars; a private bucket is recommended
	// (reads go through short-lived presigned URLs).
	PurposeAvatars = "avatars"
	// PurposeSiteAssets holds site branding (logo, favicon) referenced from
	// public pages; a public-read bucket (public base URL set) is recommended.
	PurposeSiteAssets = "site-assets"
)

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = channel.Catalog{PurposeAvatars, PurposeSiteAssets}

// ErrNotConfigured signals that the purpose is unbound or its profile is
// incomplete; callers map it to a friendly "storage not configured" RPC error.
var ErrNotConfigured = fmt.Errorf("file storage is not configured")

// ObjectStore is the app-facing surface: write via presigned PUT, read via
// ResolveURL (public URL or signed GET depending on the profile), reclaim via
// Delete.
type ObjectStore interface {
	PresignPut(ctx context.Context, purpose, key, contentType string, expires time.Duration) (string, error)
	// ResolveURL turns an object key into a fetchable URL. Public profiles
	// (public base URL set) return a stable public URL; private ones return a
	// short-lived signed GET.
	ResolveURL(ctx context.Context, purpose, key string, expires time.Duration) (string, error)
	// Delete removes an object. It is idempotent: deleting a key that no longer
	// exists returns nil, so the unattached-file sweep can safely re-run after a
	// crash (ADR-0003).
	Delete(ctx context.Context, purpose, key string) error
}

type ConnectionTester interface {
	TestConnection(ctx context.Context, cfg systemcodec.StorageProfile) error
}

// storageOps is the per-provider seam: each backend implements the three
// storage verbs against its own config shape on StorageProfile. purpose
// rides along because the local driver embeds it in signed URLs (the HTTP
// handler re-resolves purpose → profile → directory at request time, so
// rebinding a purpose never leaves stale URLs pointing at the wrong
// directory).
type storageOps struct {
	presignPut func(c *Client, ctx context.Context, cfg systemcodec.StorageProfile, purpose, key, contentType string, expires time.Duration) (string, error)
	resolveURL func(c *Client, ctx context.Context, cfg systemcodec.StorageProfile, purpose, key string, expires time.Duration) (string, error)
	delete     func(c *Client, ctx context.Context, cfg systemcodec.StorageProfile, purpose, key string) error
	test       func(c *Client, ctx context.Context, cfg systemcodec.StorageProfile) error
}

var drivers = channel.Registry[systemcodec.StorageProfile, storageOps]{
	"s3": {
		Usable: func(p systemcodec.StorageProfile) bool {
			return p.S3.Endpoint != "" && p.S3.Bucket != "" && p.S3.AccessKeyId != ""
		},
		Ops: storageOps{
			presignPut: (*Client).s3PresignPut,
			resolveURL: (*Client).s3ResolveURL,
			delete:     (*Client).s3Delete,
			test:       (*Client).s3Test,
		},
	},
	"local": {
		Usable: func(systemcodec.StorageProfile) bool { return true },
		Ops: storageOps{
			presignPut: (*Client).localPresignPut,
			resolveURL: (*Client).localResolveURL,
			delete:     (*Client).localDelete,
			test:       (*Client).localTest,
		},
	},
}

// Providers lists the registered driver names, sorted — contract-tested
// against the proto `in:` constraint in providers_test.go.
func Providers() []string {
	return drivers.Names()
}

type Client struct {
	store *settings.Store
}

func NewClient(store *settings.Store) *Client {
	return &Client{store: store}
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
	return ops.presignPut(c, ctx, cfg, purpose, key, contentType, expires)
}

func (c *Client) ResolveURL(ctx context.Context, purpose, key string, expires time.Duration) (string, error) {
	ops, cfg, err := c.opsFor(ctx, purpose)
	if err != nil {
		return "", err
	}
	return ops.resolveURL(c, ctx, cfg, purpose, key, expires)
}

func (c *Client) Delete(ctx context.Context, purpose, key string) error {
	ops, cfg, err := c.opsFor(ctx, purpose)
	if err != nil {
		return err
	}
	return ops.delete(c, ctx, cfg, purpose, key)
}

func (c *Client) TestConnection(ctx context.Context, cfg systemcodec.StorageProfile) error {
	ops, ok := drivers.OpsFor(cfg)
	if !ok {
		return ErrNotConfigured
	}
	return ops.test(c, ctx, cfg)
}

func (c *Client) opsFor(ctx context.Context, purpose string) (storageOps, systemcodec.StorageProfile, error) {
	st, err := c.store.Storage(ctx)
	if err != nil {
		return storageOps{}, systemcodec.StorageProfile{}, err
	}
	cfg, ok := st.ProfileFor(purpose)
	if !ok {
		return storageOps{}, cfg, ErrNotConfigured
	}
	ops, ok := drivers.OpsFor(cfg)
	if !ok {
		return storageOps{}, cfg, ErrNotConfigured
	}
	return ops, cfg, nil
}
