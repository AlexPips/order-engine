package telemetry

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health provides liveness and readiness check handlers.
type Health struct {
	pool *pgxpool.Pool
}

// NewHealth returns a Health checker backed by the given DB pool.
func NewHealth(pool *pgxpool.Pool) *Health {
	return &Health{pool: pool}
}

// LivenessHandler always returns 200. Use for k8s liveness probes.
func (h *Health) LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ReadinessHandler pings the DB. Returns 200 if healthy, 503 otherwise.
func (h *Health) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "no database pool", http.StatusServiceUnavailable)
		return
	}
	if err := h.pool.Ping(r.Context()); err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Ready returns true if the database is reachable. Used for programmatic checks.
func (h *Health) Ready(ctx context.Context) bool {
	if h.pool == nil {
		return false
	}
	return h.pool.Ping(ctx) == nil
}
