package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/imbytecat/moonbase/server/internal/buildinfo"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db Pinger
}

func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	dbStatus := "up"
	code := http.StatusOK
	if err := h.db.Ping(ctx); err != nil {
		status = "degraded"
		dbStatus = "down"
		code = http.StatusServiceUnavailable
	}

	build := buildinfo.Get()
	writeJSON(w, code, map[string]string{
		"status":   status,
		"database": dbStatus,
		"version":  build.Version,
		"revision": build.Revision,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
