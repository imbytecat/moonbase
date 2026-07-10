package storage

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	storageint "github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

// maxLocalObjectSize caps local uploads; presign RPCs already validate
// content_length per purpose, this is the transport-level backstop.
const maxLocalObjectSize = 32 << 20

// Handler serves the local driver's signed URLs: GET streams an object, PUT
// stores one. Signatures are verified against the same purpose/key/expiry the
// URL was issued for, and the purpose is re-resolved to its bound profile at
// request time.
type Handler struct {
	store  *settings.Store
	client *Client
	logger *slog.Logger
}

func NewHandler(store *settings.Store, client *Client, logger *slog.Logger) *Handler {
	return &Handler{store: store, client: client, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	purpose := r.PathValue("purpose")
	key := r.PathValue("key")

	// Public purposes skip the GET signature check (visibility is a static
	// property of the purpose); PUT always requires a signature — writes need
	// credentials regardless of visibility.
	public := r.Method == http.MethodGet && VisibilityOf(purpose) == VisibilityPublic
	if !public {
		exp, err := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
		if err != nil {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
		secret, err := h.store.StorageSignKey(r.Context())
		if err != nil {
			h.internal(w, r, "load sign key", err)
			return
		}
		if !storageint.VerifySignature(
			secret,
			r.Method,
			purpose,
			key,
			exp,
			r.URL.Query().Get("sig"),
		) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
	}

	st, err := h.store.Storage(r.Context())
	if err != nil {
		h.internal(w, r, "load storage settings", err)
		return
	}
	cfg, ok := st.ProfileFor(purpose)
	if !ok || cfg.Provider != "local" {
		http.Error(w, "storage not configured", http.StatusNotFound)
		return
	}
	path, err := h.client.ObjectPath(cfg, key)
	if err != nil {
		http.Error(w, "invalid object key", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if public {
			// Files are spiritually immutable (ADR-0003: replace = new file),
			// so a year-long immutable cache is sound.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		h.get(w, r, path)
	case http.MethodPut:
		h.put(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, path string) {
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
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request, path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		h.internal(w, r, "create object directory", err)
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		h.internal(w, r, "create object", err)
		return
	}
	_, copyErr := io.Copy(f, http.MaxBytesReader(w, r.Body, maxLocalObjectSize))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		h.internal(w, r, "write object", errors.Join(copyErr, closeErr))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.logger.ErrorContext(r.Context(), "local storage request failed", "op", op, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
