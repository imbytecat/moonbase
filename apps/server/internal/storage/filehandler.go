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

	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

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

	if VisibilityOf(file.Purpose) != VisibilityPublic {
		http.Error(w, "forbidden", http.StatusForbidden)
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
		h.serveLocal(w, r, cfg.Local, file)
	default:
		h.redirect(w, r, cfg, file)
	}
}

// serveLocal streams the bytes directly — a 302 back to the same server is a
// pointless round trip. Files are spiritually immutable (ADR-0003), so the
// year-long immutable cache is sound.
func (h *FileHandler) serveLocal(w http.ResponseWriter, r *http.Request, cfg systemcodec.LocalStorageConfig, file repository.File) {
	path, err := localObjectPath(cfg, file.ObjectKey)
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
func (h *FileHandler) redirect(w http.ResponseWriter, r *http.Request, cfg systemcodec.StorageProfile, file repository.File) {
	u, err := h.client.ResolveURL(r.Context(), file.Purpose, file.ObjectKey, time.Hour)
	if err != nil {
		h.internal(w, r, "resolve url", err)
		return
	}
	if cfg.S3.PublicBaseUrl != "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (h *FileHandler) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.logger.ErrorContext(r.Context(), "file request failed", "op", op, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
