package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestLivenessHandlerReturns200(t *testing.T) {
	h := &Health{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	h.LivenessHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "ok" {
		t.Fatalf("expected body 'ok', got %q", body)
	}
}

func TestReadinessHandlerReturns503WithoutDB(t *testing.T) {
	h := &Health{pool: nil}
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	h.ReadinessHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestMetricsHandlerServesPrometheus(t *testing.T) {
	m := NewMetrics()
	// Increment a counter to verify metrics are registered
	m.OrdersReceived.Inc()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "orders_received_total") {
		t.Fatalf("expected metrics output to contain orders_received_total, got:\n%s", body)
	}
}
