package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	kitsettings "github.com/imbytecat/moonbase/server/integrationkit/settings"
)

// The local driver stores objects on the server's own filesystem. There is
// no external service to presign against, so it issues application URLs
// (/api/files/...) signed with an HMAC secret from the settings store — the
// same write-without-proxying contract as S3, served by Handler.

// DefaultLocalDirectory backs local profiles with an empty directory field.
const DefaultLocalDirectory = "data/storage"

func localDir(config map[string]any) string {
	directory := cfgStr(config, "directory")
	if directory == "" {
		return DefaultLocalDirectory
	}
	return directory
}

func (c *Client) localPresignPut(ctx context.Context, _ kitsettings.GenericProfile, purpose, key, _ string, expires time.Duration) (string, error) {
	return c.localSignedURL(ctx, "PUT", purpose, key, expires)
}

// localResolveURL returns an unsigned stable URL for public purposes (the
// handler serves public GETs without a signature) and a short-lived signed
// URL for private ones.
func (c *Client) localResolveURL(ctx context.Context, _ kitsettings.GenericProfile, purpose, key string, expires time.Duration) (string, error) {
	if VisibilityOf(purpose) == VisibilityPublic {
		return "/api/files/" + purpose + "/" + key, nil
	}
	return c.localSignedURL(ctx, "GET", purpose, key, expires)
}

// localDelete removes the on-disk object. A missing file is not an error, so
// the sweep is idempotent under crash-resume (ADR-0003).
func (c *Client) localDelete(_ context.Context, cfg kitsettings.GenericProfile, _, key string) error {
	path, err := localObjectPath(cfg.Config, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// localTest proves the directory is writable by round-tripping a probe file.
func (c *Client) localTest(_ context.Context, cfg kitsettings.GenericProfile) error {
	dir := localDir(cfg.Config)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return fmt.Errorf("write to directory %q: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("write to directory %q: %w", dir, err)
	}
	return os.Remove(name)
}

func (c *Client) localSignedURL(ctx context.Context, method, purpose, key string, expires time.Duration) (string, error) {
	secret, err := c.store.StorageSignKey(ctx)
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(expires).Unix()
	q := url.Values{
		"exp": {strconv.FormatInt(exp, 10)},
		"sig": {localSignature(secret, method, purpose, key, exp)},
	}
	return "/api/files/" + purpose + "/" + key + "?" + q.Encode(), nil
}

func localSignature(secret []byte, method, purpose, key string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	// hash.Hash.Write never returns an error per its contract.
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%s\n%d", method, purpose, key, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyLocalSignature(secret []byte, method, purpose, key string, exp int64, sig string) bool {
	if time.Now().Unix() > exp {
		return false
	}
	expected := localSignature(secret, method, purpose, key, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// localObjectPath maps an object key into the profile directory, rejecting
// path traversal: the cleaned path must stay inside the directory.
func localObjectPath(config map[string]any, key string) (string, error) {
	dir := localDir(config)
	path := filepath.Join(dir, filepath.FromSlash(key))
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("invalid object key %q", key)
	}
	return path, nil
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}
