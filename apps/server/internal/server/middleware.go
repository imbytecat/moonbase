// Hand-rolled net/http middleware — small enough that a router framework
// would cost more than it saves. Both emit structured slog, so the whole
// server has exactly one log pipeline (JSON on stderr).
package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// statusWriter records the status code for the access log. Unwrap keeps
// http.ResponseController (flush/hijack/deadlines) working through the wrap;
// Flush stays for code that type-asserts http.Flusher directly (streaming RPCs).
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// accessLog logs one line per request.
func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
		)
	})
}

// recoverer converts handler panics into logged 500s. http.ErrAbortHandler is
// net/http's own abort sentinel and must keep propagating.
func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Identity comparison is how net/http itself detects this
				// sentinel; it is never wrapped.
				if rec == http.ErrAbortHandler { //nolint:errorlint
					panic(rec)
				}
				logger.Error("panic", "error", rec, "stack", string(debug.Stack()))
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders is XSS/clickjacking defense-in-depth for the embedded SPA.
// httpOnly cookies stop token THEFT, but during an active XSS the attacker can
// still ride the session — CSP shrinks the odds of XSS existing at all.
func securityHeaders(secureCookie bool, next http.Handler) http.Handler {
	// Vite emits plain JS + hashed asset files; no inline scripts. antd/Tailwind
	// inject <style> at runtime, hence style-src 'unsafe-inline' (standard for
	// CSS-in-JS; the dangerous vector is scripts, which stay locked down).
	// connect-src 'self' covers RPC + presigned S3 only when same-origin, so
	// external S3 endpoints get an explicit pass via blob/data for previews.
	csp := "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob: https: http:; " +
		"connect-src 'self' https: http:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if secureCookie {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// RPC responses can carry PII; never let shared caches keep them.
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
