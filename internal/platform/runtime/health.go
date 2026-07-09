package runtime

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// Health exposes uniform readiness and version responses for a service.
type Health struct {
	serviceName string
	version     string
	commit      string
	date        string
	startedAt   time.Time
	ready       atomic.Bool
}

// NewHealth builds a new health state container.
func NewHealth(serviceName, version, commit, date string) *Health {
	health := &Health{
		serviceName: serviceName,
		version:     version,
		commit:      commit,
		date:        date,
		startedAt:   time.Now().UTC(),
	}
	return health
}

// SetReady marks the service as ready to receive traffic.
func (h *Health) SetReady() {
	h.ready.Store(true)
}

// SetNotReady marks the service as not ready to receive traffic.
func (h *Health) SetNotReady() {
	h.ready.Store(false)
}

// Ready reports whether the service is ready.
func (h *Health) Ready() bool {
	return h.ready.Load()
}

// Version returns the build version.
func (h *Health) Version() string {
	return h.version
}

// Commit returns the build commit.
func (h *Health) Commit() string {
	return h.commit
}

// Date returns the build date.
func (h *Health) Date() string {
	return h.date
}

// Handler wraps the provided handler with health endpoints.
func (h *Health) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			h.writeStatus(w, http.StatusOK, "ok")
			return
		case "/readyz":
			if !h.Ready() {
				h.writeStatus(w, http.StatusServiceUnavailable, "not_ready")
				return
			}

			h.writeStatus(w, http.StatusOK, "ready")
			return
		case "/version":
			h.writeVersion(w)
			return
		default:
			if next == nil {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		}
	})
}

func (h *Health) writeStatus(w http.ResponseWriter, statusCode int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"service":    h.serviceName,
		"status":     state,
		"version":    h.version,
		"commit":     h.commit,
		"build_date": h.date,
		"started_at": h.startedAt.Format(time.RFC3339Nano),
	})
}

func (h *Health) writeVersion(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"service":    h.serviceName,
		"version":    h.version,
		"commit":     h.commit,
		"build_date": h.date,
	})
}
