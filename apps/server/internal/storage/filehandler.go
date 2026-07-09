package storage

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	storageint "github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// privateURLTTL bounds how long a private file's signed URL stays valid after
// the authenticated 302.
const privateURLTTL = 5 * time.Minute

// FileHandler serves GET /f/{file_id}, the permanent file URL (ADR-0004):
// every RPC returns this shape and the handler dispatches on the purpose's
// visibility × the bound profile's driver, so storage URLs stay an internal
// implementation detail.
type FileHandler struct {
	store  *settings.Store
	client *Client
	repo   repository.Querier
	logger *slog.Logger
}

func NewFileHandler(store *settings.Store, client *Client, repo repository.Querier, logger *slog.Logger) *FileHandler {
	return &FileHandler{store: store, client: client, repo: repo, logger: logger}
}

func (h *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("file_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := h.repo.GetFile(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.internal(w, r, "get file", err)
		return
	}

	// private purpose: session auth first (the /f/ route is wrapped by the
	// authn middleware, which resolves but never rejects), then redirect to a
	// short-lived signed URL. The 302 must never be cached — its target
	// carries an expiry, so a cached redirect would bypass the auth window.
	if VisibilityOf(file.Purpose) != VisibilityPublic {
		if auth.IdentityFromContext(r.Context()) == nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		u, err := h.client.ResolveURL(r.Context(), file.Purpose, file.ObjectKey, privateURLTTL)
		if err != nil {
			h.internal(w, r, "resolve signed url", err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(w, r, u, http.StatusFound)
		return
	}

	st, err := h.store.Storage(r.Context())
	if err != nil {
		h.internal(w, r, "load storage settings", err)
		return
	}
	cfg, ok := st.ProfileFor(file.Purpose)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch cfg.Provider {
	case "local":
		h.serveLocal(w, r, cfg.Config, file)
	default:
		h.redirect(w, r, cfg, file)
	}
}

// serveLocal streams the bytes directly — a 302 back to the same server is a
// pointless round trip. Files are spiritually immutable (ADR-0003), so the
// year-long immutable cache is sound.
func (h *FileHandler) serveLocal(w http.ResponseWriter, r *http.Request, config map[string]any, file repository.File) {
	path, err := storageint.LocalObjectPath(config, file.ObjectKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.internal(w, r, "open object", err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		h.internal(w, r, "stat object", err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if file.ContentType != "" {
		w.Header().Set("Content-Type", file.ContentType)
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

// redirect sends the client to the driver's URL. A stable public URL may be
// cached briefly (rebinding a profile leaves stale redirects alive for at
// most an hour); a signed URL carries an expiry, so caching the redirect
// would outlive it — never store those.
func (h *FileHandler) redirect(w http.ResponseWriter, r *http.Request, cfg kitsettings.GenericProfile, file repository.File) {
	u, err := h.client.ResolveURL(r.Context(), file.Purpose, file.ObjectKey, time.Hour)
	if err != nil {
		h.internal(w, r, "resolve url", err)
		return
	}
	if publicBaseURL(cfg.Config) != "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func publicBaseURL(config map[string]any) string {
	s, _ := config["publicBaseUrl"].(string)
	return s
}

func (h *FileHandler) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.logger.ErrorContext(r.Context(), "file request failed", "op", op, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
