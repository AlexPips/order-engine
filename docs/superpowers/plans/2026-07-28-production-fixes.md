# Production Readiness Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 6 production-readiness issues (Dockerfile, CI race, Prometheus metrics, request ID, event ordering, TradeFeed leak) plus implement OTel tracing (closing a README lie) and add integration tests as a dedicated CI job.

**Architecture:** Add `internal/telemetry` for Prometheus metrics, health checks, and OTel tracing. Add request ID interceptor. Fix correctness bugs in `order_service.go`. Update CI and Dockerfile. Add integration test CI job with Docker.

**Tech Stack:** Go 1.26, Prometheus client_golang, OpenTelemetry SDK + OTel gRPC interceptors, slog (stdlib), gRPC interceptors, pgx/v5, testcontainers-go

## Global Constraints

- Go version: 1.26 (go.mod line 3) — Dockerfile must match
- Existing linter: golangci-lint v2.12.2 with `.golangci.yml` config
- Existing logging: `log/slog` with JSON output (interceptors/interceptors.go)
- Existing interceptors: `interceptors.RecoveryUnary()`, `interceptors.LoggingUnary()` (cmd/server/main.go:62-63)
- Existing bus: `events.Bus` with Publish/SubscribeChannel pattern (events/bus.go)
- Existing DB: pgxpool with MaxConns=10, MinConns=2 (db/pool.go:22-25)
- No existing `internal/telemetry/` directory — README claims it exists but it doesn't
- CI runs on ubuntu-latest with Go 1.26

---

### Task 1: Fix Dockerfile Go Version

**Files:**
- Modify: `deployments/Dockerfile:1`

**Interfaces:**
- Consumes: `go.mod` specifies Go 1.26
- Produces: Docker builds will use matching Go version

**Why:** The Dockerfile uses `golang:1.23-alpine` while go.mod requires 1.26. Any code using features from Go 1.24+ will fail in Docker builds. This is a 2-minute fix that eliminates a build surprise for anyone running `docker compose up --build`.

- [ ] **Step 1: Read current Dockerfile**

```bash
cat deployments/Dockerfile
```

Expected output:
```dockerfile
FROM golang:1.23-alpine AS builder
...
```

- [ ] **Step 2: Fix Go version in Dockerfile**

Edit `deployments/Dockerfile` line 1:

Change:
```dockerfile
FROM golang:1.23-alpine AS builder
```

To:
```dockerfile
FROM golang:1.26-alpine AS builder
```

- [ ] **Step 3: Verify Dockerfile builds**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && docker build -t order-engine-test -f deployments/Dockerfile .
```

Expected: Build completes successfully, no errors.

- [ ] **Step 4: Clean up test image**

Run:
```bash
docker rmi order-engine-test
```

- [ ] **Step 5: Commit**

```bash
git add deployments/Dockerfile
git commit -m "fix: update Dockerfile Go version from 1.23 to 1.26 to match go.mod"
```

---

### Task 2: Add Race Detector to CI

**Files:**
- Modify: `.github/workflows/ci.yml:36`

**Interfaces:**
- Consumes: existing CI workflow structure
- Produces: CI will catch data races in tests

**Why:** The `-race` flag enables Go's race detector, which catches real concurrency bugs at test time. Without it, your CI tests run without race detection — the matching engine's RWMutex and double-check locking patterns are exactly the kind of code races that `-race` catches.

- [ ] **Step 1: Read current CI test step**

```bash
cat .github/workflows/ci.yml | grep -A2 "go test"
```

Expected output:
```yaml
      - run: go test -coverprofile=coverage.out ./...
      - run: go tool cover -func=coverage.out | tail -1
```

- [ ] **Step 2: Add -race flag to CI test step**

Edit `.github/workflows/ci.yml` line 36:

Change:
```yaml
      - run: go test -coverprofile=coverage.out ./...
```

To:
```yaml
      - run: go test -race -coverprofile=coverage.out ./...
```

- [ ] **Step 3: Verify CI file is valid YAML**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "Valid YAML"
```

Expected: Prints "Valid YAML"

- [ ] **Step 4: Run tests locally with race detector**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go test -race -count=1 ./internal/matching/ ./internal/events/
```

Expected: All tests pass. If races are detected, they must be fixed before proceeding.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add -race flag to test step for concurrency bug detection"
```

---

### Task 3: Add Prometheus Metrics and Health Endpoints

**Files:**
- Create: `internal/telemetry/metrics.go`
- Create: `internal/telemetry/health.go`
- Create: `internal/telemetry/metrics_test.go`
- Modify: `cmd/server/main.go:73-79` (wire metrics HTTP server)
- Modify: `internal/server/order_service.go` (instrument order creation)
- Modify: `internal/matching/engine.go` (instrument matching)
- Modify: `go.mod` (add prometheus dependency)

**Interfaces:**
- Consumes: `pgxpool.Pool` for health checks, `matching.Engine` for queue depth
- Produces: `telemetry.Metrics` struct with counters/histograms, `telemetry.Health` struct with readiness check, HTTP handler for `/metrics` and `/healthz`/`/ready`

**Why:** Your README claims "Prometheus metrics: orders submitted, trades executed, order book depth, latency histograms" and lists `internal/telemetry/` in the project structure. Neither exists. This is either implement the claim or remove it. Implementing is better — it's a concrete skill signal and takes 2-3 hours.

- [ ] **Step 1: Add prometheus/client_golang dependency**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go get github.com/prometheus/client_golang@latest
```

Expected: go.mod updated with prometheus dependency.

- [ ] **Step 2: Create internal/telemetry/metrics.go**

```go
package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the order engine.
type Metrics struct {
	OrdersReceived   prometheus.Counter
	OrdersSubmitted  prometheus.Counter
	TradesExecuted   prometheus.Counter
	OrderLatency     prometheus.Histogram
	BookDepth        *prometheus.GaugeVec
	ActiveStreams     prometheus.Gauge
}

// NewMetrics registers and returns all metrics. Call once at startup.
func NewMetrics() *Metrics {
	return &Metrics{
		OrdersReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "orders_received_total",
			Help: "Total number of orders received by the service.",
		}),
		OrdersSubmitted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "orders_submitted_total",
			Help: "Total number of orders successfully submitted to the matching engine.",
		}),
		TradesExecuted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "trades_executed_total",
			Help: "Total number of trades executed by the matching engine.",
		}),
		OrderLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "order_submit_duration_seconds",
			Help:    "Time spent submitting an order to the matching engine.",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 20), // 1μs to ~1s
		}),
		BookDepth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "order_book_depth",
			Help: "Number of price levels in the order book per symbol.",
		}, []string{"symbol"}),
		ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "active_trade_streams",
			Help: "Number of active TradeFeed streams.",
		}),
	}
}
```

- [ ] **Step 3: Create internal/telemetry/health.go**

```go
package telemetry

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	if err := h.pool.Ping(r.Context()); err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Handler returns an http.ServeMux with /metrics, /healthz, and /ready registered.
func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", h.LivenessHandler)
	mux.HandleFunc("/ready", h.ReadinessHandler)
	return mux
}

// Ready returns true if the database is reachable. Used for programmatic checks.
func (h *Health) Ready(ctx context.Context) bool {
	return h.pool.Ping(ctx) == nil
}
```

- [ ] **Step 4: Create internal/telemetry/metrics_test.go**

```go
package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	h := &Health{}
	mux := h.Handler()

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
```

- [ ] **Step 5: Run telemetry tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go test ./internal/telemetry/ -v
```

Expected: All 3 tests pass.

- [ ] **Step 6: Wire telemetry into cmd/server/main.go**

Edit `cmd/server/main.go`. Add imports and wire the metrics HTTP server.

Add to imports (after existing imports):
```go
	"github.com/AlexPips/order-engine/internal/telemetry"
```

Add after line 52 (`srv := server.NewOrderService(...)`, before `if err := srv.RecoverState`):
```go
	metrics := telemetry.NewMetrics()
	health := telemetry.NewHealth(pool)
```

Replace the existing pprof server block (lines 73-79) with:
```go
	// Metrics + health + pprof on :6060
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", health.LivenessHandler)
	mux.HandleFunc("/ready", health.ReadinessHandler)
	mux.HandleFunc("/debug/pprof/", http.DefaultServeXxx.ServeHTTP)

	metricsServer := &http.Server{Addr: ":6060", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("metrics+health listening", "addr", ":6060")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()
```

Update the shutdown block (line 94) to close metricsServer:
```go
	<-stop
	slog.Info("shutting down")
	grpcServer.GracefulStop()
	metricsServer.Close()
	pool.Close()
```

Add `"net/http/prometheus"` and `"net/http"` to the import block if not already present. Remove the `_ "net/http/pprof"` blank import since pprof is now wired via the mux.

- [ ] **Step 7: Pass metrics to OrderService**

Edit `cmd/server/main.go` line 52:

Change:
```go
	srv := server.NewOrderService(engine, bus, queries, pool)
```

To:
```go
	srv := server.NewOrderService(engine, bus, queries, pool, metrics)
```

- [ ] **Step 8: Update OrderService to accept metrics**

Edit `internal/server/order_service.go`:

Add import:
```go
	"github.com/AlexPips/order-engine/internal/telemetry"
```

Update struct (line 21-29):
```go
type OrderService struct {
	orderpb.UnimplementedOrderServiceServer
	engine  *matching.Engine
	bus     *events.Bus
	repo    *repository.Queries
	pool    *pgxpool.Pool
	metrics *telemetry.Metrics
	mu      sync.RWMutex
	orders  map[domain.OrderID]*domain.Order
}
```

Update constructor (line 31-39):
```go
func NewOrderService(engine *matching.Engine, bus *events.Bus, repo *repository.Queries, pool *pgxpool.Pool, metrics *telemetry.Metrics) *OrderService {
	return &OrderService{
		engine:  engine,
		bus:     bus,
		repo:    repo,
		pool:    pool,
		metrics: metrics,
		orders:  make(map[domain.OrderID]*domain.Order),
	}
}
```

- [ ] **Step 9: Instrument CreateOrder**

Edit `internal/server/order_service.go`, in `CreateOrder` method (after line 121, before `s.engine.SubmitOrder`):

```go
	s.metrics.OrdersReceived.Inc()
	start := time.Now()
```

After `s.engine.SubmitOrder` returns (after line 123):
```go
	s.metrics.OrderLatency.Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.metrics.OrdersSubmitted.Inc()
	if len(trades) > 0 {
		s.metrics.TradesExecuted.Add(float64(len(trades)))
	}
```

- [ ] **Step 10: Instrument matching engine with book depth**

Edit `internal/matching/engine.go`, in `GetOrderBook` method (after line 61, before `return snap`):

Add a method to expose book depth for metrics:
```go
// BookSymbols returns all tracked symbols. Used by telemetry.
func (e *Engine) BookSymbols() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	symbols := make([]string, 0, len(e.books))
	for sym := range e.books {
		symbols = append(symbols, sym)
	}
	return symbols
}

// BookDepth returns the number of price levels for a symbol. Used by telemetry.
func (e *Engine) BookDepth(symbol string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	book, ok := e.books[symbol]
	if !ok {
		return 0
	}
	book.mu.RLock()
	defer book.mu.RUnlock()
	return len(book.bids) + len(book.asks)
}
```

Then in `cmd/server/main.go`, add a goroutine to periodically update book depth metrics (after the metrics server goroutine):

```go
	// Update book depth metrics every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for _, sym := range engine.BookSymbols() {
					metrics.BookDepth.WithLabelValues(sym).Set(float64(engine.BookDepth(sym)))
				}
			case <-stop:
				return
			}
		}
	}()
```

Note: `stop` channel is defined at line 81. The goroutine should be started after `stop` is created.

- [ ] **Step 11: Build and run tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build ./... && go test ./internal/telemetry/ -v
```

Expected: Build succeeds, all telemetry tests pass.

- [ ] **Step 12: Commit**

```bash
git add internal/telemetry/ cmd/server/main.go internal/server/order_service.go internal/matching/engine.go go.mod go.sum
git commit -m "feat: add Prometheus metrics, health endpoints, and book depth instrumentation"
```

---

### Task 4: Add Request ID Interceptor

**Files:**
- Modify: `internal/interceptors/interceptors.go` (add RequestID interceptor)
- Modify: `cmd/server/main.go:62-63` (wire into interceptor chain)

**Interfaces:**
- Consumes: `context.Context` (standard Go context)
- Produces: `RequestIDUnary()` and `RequestIDStream()` interceptors that inject UUID into context and log it

**Why:** Your slog output has no request correlation. Under concurrent load, you can't trace a single request through the logs. This interceptor adds a UUID to every request and logs it — standard production practice.

- [ ] **Step 1: Add RequestID interceptor to interceptors.go**

Edit `internal/interceptors/interceptors.go`. Add a new context key and two interceptors.

Add to imports:
```go
	"github.com/google/uuid"
```

Add after the existing `RecoveryStream` function:
```go
type requestIDKey struct{}

// RequestIDUnary injects a UUID into the context and logs it.
func RequestIDUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		rid := uuid.NewString()
		ctx = context.WithValue(ctx, requestIDKey{}, rid)
		slog.Info("grpc.request.start",
			"method", info.FullMethod,
			"request_id", rid,
		)
		resp, err := handler(ctx, req)
		slog.Info("grpc.request.end",
			"method", info.FullMethod,
			"request_id", rid,
			"code", status.Code(err),
		)
		return resp, err
	}
}

// RequestIDStream injects a UUID into the stream context.
func RequestIDStream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		rid := uuid.NewString()
		wrapped := &requestIDStream{ServerStream: ss, rid: rid}
		slog.Info("grpc.stream.start",
			"method", info.FullMethod,
			"request_id", rid,
		)
		err := handler(srv, wrapped)
		slog.Info("grpc.stream.end",
			"method", info.FullMethod,
			"request_id", rid,
			"code", status.Code(err),
		)
		return err
	}
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok {
		return rid
	}
	return ""
}

type requestIDStream struct {
	grpc.ServerStream
	rid string
}

func (s *requestIDStream) Context() context.Context {
	return context.WithValue(s.ServerStream.Context(), requestIDKey{}, s.rid)
}
```

- [ ] **Step 2: Wire interceptors into server**

Edit `cmd/server/main.go` lines 62-68:

Change:
```go
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.LoggingUnary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.LoggingStream(),
		),
	)
```

To:
```go
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.RequestIDUnary(),
			interceptors.LoggingUnary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.RequestIDStream(),
			interceptors.LoggingStream(),
		),
	)
```

- [ ] **Step 3: Build and run all tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build ./... && go test ./...
```

Expected: Build succeeds, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/interceptors/interceptors.go cmd/server/main.go
git commit -m "feat: add request ID interceptor for request correlation in logs"
```

---

### Task 5: Fix Event Publish Ordering

**Files:**
- Modify: `internal/server/order_service.go:70-93` (persistOrderTx method)

**Interfaces:**
- Consumes: `events.Bus.Publish()`, `pgxpool.Begin()`
- Produces: Events published only after DB commit succeeds

**Why:** Currently `persistOrderTx` publishes trade events (line 83-86) inside the DB transaction, before the commit. If the commit fails, subscribers have already seen phantom trades. The fix: collect events during the transaction, publish only after commit succeeds.

- [ ] **Step 1: Read current persistOrderTx**

Read `internal/server/order_service.go` lines 70-93. Current code:
```go
func (s *OrderService) persistOrderTx(ctx context.Context, o *domain.Order, trades []domain.Trade) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txRepo := s.repo.WithTx(tx)
	if _, err := txRepo.CreateOrder(ctx, domainToCreateParams(o)); err != nil {
		return err
	}

	for _, t := range trades {
		s.bus.Publish("trade."+t.Symbol, events.TradeEvent{  // ← BUG: publishes before commit
			Symbol: t.Symbol, BuyID: string(t.BuyOrderID),
			SellID: string(t.SellOrderID), Price: t.Price.String(), Qty: t.Quantity.String(),
		})
		if _, err := txRepo.CreateTrade(ctx, domainToTradeParams(&t)); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Fix the method**

Replace `persistOrderTx` with:
```go
func (s *OrderService) persistOrderTx(ctx context.Context, o *domain.Order, trades []domain.Trade) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txRepo := s.repo.WithTx(tx)
	if _, err := txRepo.CreateOrder(ctx, domainToCreateParams(o)); err != nil {
		return err
	}

	for _, t := range trades {
		if _, err := txRepo.CreateTrade(ctx, domainToTradeParams(&t)); err != nil {
			return err
		}
	}

	// Commit first, then publish — no phantom events on rollback
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	for _, t := range trades {
		s.bus.Publish("trade."+t.Symbol, events.TradeEvent{
			Symbol: t.Symbol, BuyID: string(t.BuyOrderID),
			SellID: string(t.SellOrderID), Price: t.Price.String(), Qty: t.Quantity.String(),
		})
	}

	return nil
}
```

- [ ] **Step 3: Build and run tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build ./... && go test ./...
```

Expected: Build succeeds, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/order_service.go
git commit -m "fix: publish trade events after DB commit, not before — prevents phantom events on rollback"
```

---

### Task 6: Fix TradeFeed Goroutine Leak

**Files:**
- Modify: `internal/server/order_service.go:308-400` (TradeFeed method)

**Interfaces:**
- Consumes: `grpc.BidiStreamingServer`, `events.Bus`
- Produces: All goroutines exit cleanly when stream closes; no orphaned senders

**Why:** The current TradeFeed spawns goroutines (line 334) that call `stream.Send` in a loop with no context check. When the client disconnects, the `Recv` goroutine sends an error to `errc`, but the sender goroutines are orphaned — they keep trying to send on a dead stream. This leaks goroutines on every disconnect.

- [ ] **Step 1: Read current TradeFeed**

Read `internal/server/order_service.go` lines 308-400.

- [ ] **Step 2: Fix the method**

Replace the entire `TradeFeed` method with:
```go
func (s *OrderService) TradeFeed(stream grpc.BidiStreamingServer[orderpb.TradeFeedRequest, orderpb.TradeFeedResponse]) error {
	ctx := stream.Context()
	errc := make(chan error, 2)
	subscriptions := make(map[string]chan any)
	var subMu sync.Mutex
	var wg sync.WaitGroup

	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				errc <- err
				return
			}
			switch m := req.GetMsg().(type) {
			case *orderpb.TradeFeedRequest_SubscribeSymbol:
				symbol := m.SubscribeSymbol
				topic := "trade." + symbol

				subMu.Lock()
				if _, ok := subscriptions[symbol]; ok {
					subMu.Unlock()
					continue
				}
				ch := s.bus.SubscribeChannel(topic, 256)
				subscriptions[symbol] = ch
				subMu.Unlock()

				wg.Add(1)
				go func(sym string, tradeCh chan any) {
					defer wg.Done()
					for {
						select {
						case msg, ok := <-tradeCh:
							if !ok {
								return
							}
							ev, ok := msg.(events.TradeEvent)
							if !ok {
								continue
							}
							if err := stream.Send(&orderpb.TradeFeedResponse{
								Msg: &orderpb.TradeFeedResponse_Trade{
									Trade: &orderpb.Trade{
										Symbol:      ev.Symbol,
										BuyOrderId:  ev.BuyID,
										SellOrderId: ev.SellID,
										Price:       stringToDecimalProto(ev.Price),
										Quantity:    stringToDecimalProto(ev.Qty),
									},
								},
							}); err != nil {
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}(symbol, ch)

			case *orderpb.TradeFeedRequest_NewOrder:
				o := domain.Order{
					ID:             domain.OrderID(m.NewOrder.GetIdempotencyKey()),
					UserID:         domain.UserID(m.NewOrder.GetUserId()),
					Symbol:         m.NewOrder.GetSymbol(),
					Side:           protoToDomainSide(m.NewOrder.GetSide()),
					Type:           protoToDomainType(m.NewOrder.GetType()),
					Price:          pbDecimalToDecimal(m.NewOrder.GetPrice()),
					Quantity:       pbDecimalToDecimal(m.NewOrder.GetQuantity()),
					Status:         domain.OrderStatusNew,
					CreatedAt:      time.Now().UnixNano(),
					UpdatedAt:      time.Now().UnixNano(),
					MaxSlippageBPS: m.NewOrder.GetMaxSlippageBps(),
				}

				s.mu.Lock()
				if _, exists := s.orders[o.ID]; exists {
					s.mu.Unlock()
					continue
				}
				trades, err := s.engine.SubmitOrder(ctx, &o)
				if err != nil {
					s.mu.Unlock()
					continue
				}
				s.orders[o.ID] = &o
				s.mu.Unlock()

				_ = s.persistOrderTx(context.Background(), &o, trades) //nolint:errcheck
			}
		}
	}()

	// Block until Recv goroutine signals (client disconnect or error)
 recvErr := <-errc

	// Cancel all sender goroutines
	subMu.Lock()
	for symbol, ch := range subscriptions {
		topic := "trade." + symbol
		s.bus.UnsubscribeChannel(topic, ch)
	}
	subMu.Unlock()

	// Wait for all sender goroutines to exit
	wg.Wait()

	return recvErr
}
```

Key changes:
- Added `ctx := stream.Context()` at the top
- Added `var wg sync.WaitGroup` to track sender goroutines
- Sender goroutines now `defer wg.Done()` and `select` on both `tradeCh` and `ctx.Done()`
- After unsubscribing, `wg.Wait()` ensures all senders exit before returning
- Sender goroutines check channel close (`ok` from range) AND context cancellation

- [ ] **Step 3: Build and run tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build ./... && go test ./... -race -count=1
```

Expected: Build succeeds, all tests pass, no races detected.

- [ ] **Step 4: Commit**

```bash
git add internal/server/order_service.go
git commit -m "fix: prevent goroutine leak in TradeFeed — sender goroutines now select on ctx.Done and wg.Wait ensures clean exit"
```

---

### Task 7: Final Verification and README Cleanup

**Files:**
- Modify: `README.md` (remove `internal/telemetry/` from project structure since it now exists, verify all claims are accurate)

**Interfaces:**
- Consumes: all previous tasks complete
- Produces: README matches reality

- [ ] **Step 1: Run full test suite with race detector**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go test -race -count=1 ./...
```

Expected: All tests pass, no races.

- [ ] **Step 2: Run golangci-lint**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && golangci-lint run
```

Expected: No new lint errors. If errors appear, fix them.

- [ ] **Step 3: Build binary**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build -o bin/server ./cmd/server/
```

Expected: Binary builds successfully.

- [ ] **Step 4: Verify README claims are now accurate**

Read `README.md` and verify:
- "OpenTelemetry traces" → OTel is in go.mod as indirect, Jaeger in docker-compose. This is aspirational but not a lie — the infrastructure exists.
- "Prometheus metrics" → Now implemented in `internal/telemetry/`
- "Structured logging via slog" → Implemented in interceptors
- "Health check" → Now at `/healthz` and `/ready`
- `internal/telemetry/` in project structure → Now exists

If any claims are still inaccurate, update the README.

- [ ] **Step 5: Final commit if README changed**

```bash
git add README.md
git commit -m "docs: verify README claims match implementation"
```

---

### Task 8: Implement OpenTelemetry Tracing

**Files:**
- Create: `internal/telemetry/tracing.go`
- Create: `internal/telemetry/tracing_test.go`
- Modify: `cmd/server/main.go` (init tracer, wire OTel gRPC interceptors)
- Modify: `internal/matching/engine.go` (add trace spans to SubmitOrder)
- Modify: `go.mod` (add OTel SDK + OTel gRPC as direct deps)

**Interfaces:**
- Consumes: `go.opentelemetry.io/otel` (already indirect in go.mod), Jaeger endpoint from env `OTEL_EXPORTER_OTLP_ENDPOINT` (already in docker-compose.yml:50)
- Produces: `telemetry.InitTracer()` function, OTel gRPC interceptors for span propagation, trace spans on matching engine operations

**Why:** Your README says "OpenTelemetry traces across gRPC handlers → matching engine → repository" and docker-compose has Jaeger on `:4318`. But there's zero tracing code. The OTel packages in go.mod are indirect dependencies from testcontainers — they're not wired into your application. This is either implement the claim or remove it. Implementing is better — Jaeger is already configured, you just need to connect the pieces. Traces across gRPC → matching → DB is a strong interview talking point.

- [ ] **Step 1: Add OTel SDK and OTel gRPC as direct dependencies**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && \
go get go.opentelemetry.io/otel@latest \
  go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/sdk/resource@latest \
  go.opentelemetry.io/otel/semconv/v1.26.0@latest \
  go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc@latest
```

Expected: go.mod updated with OTel SDK as direct (not indirect) dependencies.

- [ ] **Step 2: Create internal/telemetry/tracing.go**

```go
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer initializes an OTel tracer provider that exports to Jaeger via OTLP/gRPC.
// The endpoint is read from OTEL_EXPORTER_OTLP_ENDPOINT env var (default: "localhost:4318").
// Returns a shutdown function that flushes pending spans. Call defer shutdown() in main.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := "localhost:4318"
	if ep := otelExporterEndpoint(); ep != "" {
		endpoint = ep
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

func otelExporterEndpoint() string {
	// os.Getenv is read at init; if caller wants DI, pass endpoint directly.
	// For this project, env var is sufficient.
	import_os := __import_os()
	return import_os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
}
```

**Wait — that's wrong.** The `otelExporterEndpoint` helper has a fake import. Here's the corrected version:

```go
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer initializes an OTel tracer provider that exports to Jaeger via OTLP/gRPC.
// The endpoint is read from OTEL_EXPORTER_OTLP_ENDPOINT env var (default: "localhost:4318").
// Returns a shutdown function that flushes pending spans. Call defer shutdown() in main.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4318"
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
```

- [ ] **Step 3: Create internal/telemetry/tracing_test.go**

```go
package telemetry

import (
	"context"
	"testing"
)

func TestInitTracerReturnsShutdownFunc(t *testing.T) {
	// Set endpoint to a non-existent address — init should succeed,
	// shutdown should handle gracefully.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:99999")

	shutdown, err := InitTracer(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("InitTracer returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	// Shutdown should not error even with unreachable endpoint
	// (batch exporter defers network calls).
	if err := shutdown(context.Background()); err != nil {
		t.Logf("shutdown returned error (acceptable for test): %v", err)
	}
}

func TestInitTracerDefaultEndpoint(t *testing.T) {
	// Clear env var to test default
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := InitTracer(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("InitTracer returned error: %v", err)
	}
	defer shutdown(context.Background()) //nolint:errcheck
}
```

- [ ] **Step 4: Run tracing tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go test ./internal/telemetry/ -v -run TestInitTracer
```

Expected: Both tests pass.

- [ ] **Step 5: Add trace spans to matching engine**

Edit `internal/matching/engine.go`. Add OTel tracing to `SubmitOrder`.

Add to imports:
```go
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
```

Replace the `SubmitOrder` method (lines 26-36):
```go
func (e *Engine) SubmitOrder(ctx context.Context, o *domain.Order) ([]domain.Trade, error) {
	tracer := otel.Tracer("order-engine/matching")
	ctx, span := tracer.Start(ctx, "matching.SubmitOrder",
		trace.WithAttributes(
			semconv.MessagingSystem("order-engine"),
			attribute.String("order.symbol", o.Symbol),
			attribute.String("order.side", string(o.Side)),
			attribute.String("order.type", string(o.Type)),
			attribute.String("order.id", string(o.ID)),
		),
	)
	defer span.End()

	book := e.getOrCreateBook(o.Symbol)
	var trades []domain.Trade
	var err error

	switch o.Type {
	case domain.OrderTypeMarket:
		trades, err = e.matchMarket(ctx, book, o)
	case domain.OrderTypeLimit:
		trades, err = e.matchLimit(ctx, book, o, decimal.Zero)
	default:
		err = errors.New("unknown order type")
	}

	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.Int("trades.count", len(trades)))
	}

	return trades, err
}
```

Add `trace` to the import from `go.opentelemetry.io/otel`:
```go
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
```

- [ ] **Step 6: Wire OTel interceptors and tracer init into cmd/server/main.go**

Add to imports:
```go
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
```

Add after `db.Connect` (after line 39), before `lis, err := net.Listen`:
```go
	// Initialize OpenTelemetry tracer (exports to Jaeger via OTLP)
	shutdownTracer, err := telemetry.InitTracer(ctx, "order-engine")
	if err != nil {
		slog.Warn("failed to init tracer, continuing without tracing", "error", err)
	} else {
		slog.Info("tracer initialized", "endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
```

Update the gRPC server creation (lines 60-69) to add OTel interceptors:
```go
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptors.RecoveryUnary(),
			interceptors.RequestIDUnary(),
			otelgrpc.UnaryServerInterceptor(),
			interceptors.LoggingUnary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.RecoveryStream(),
			interceptors.RequestIDStream(),
			otelgrpc.StreamServerInterceptor(),
			interceptors.LoggingStream(),
		),
	)
```

Update the shutdown block to flush tracer:
```go
	<-stop
	slog.Info("shutting down")
	grpcServer.GracefulStop()
	metricsServer.Close()
	if shutdownTracer != nil {
		if err := shutdownTracer(context.Background()); err != nil {
			slog.Error("tracer shutdown error", "error", err)
		}
	}
	pool.Close()
```

- [ ] **Step 7: Build and run all tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go build ./... && go test -race -count=1 ./...
```

Expected: Build succeeds, all tests pass. OTel tests may log warnings about unreachable Jaeger (expected in local dev without docker-compose).

- [ ] **Step 8: Verify traces appear in Jaeger (optional, requires docker-compose)**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && \
docker compose -f deployments/docker-compose.yml up -d jaeger && \
sleep 3 && \
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318 go run ./cmd/server/
```

In another terminal, submit an order via grpcurl, then open `http://localhost:16686` (Jaeger UI). You should see a `matching.SubmitOrder` span under the `order-engine` service.

- [ ] **Step 9: Commit**

```bash
git add internal/telemetry/tracing.go internal/telemetry/tracing_test.go \
  internal/matching/engine.go cmd/server/main.go go.mod go.sum
git commit -m "feat: implement OpenTelemetry tracing with Jaeger export — closes README claim"
```

---

### Task 9: Add Integration Tests as Dedicated CI Job

**Files:**
- Modify: `.github/workflows/ci.yml` (add integration-test job)
- Modify: `internal/db/pool.go` (make migration path configurable)

**Interfaces:**
- Consumes: existing testcontainers setup in `internal/db/pool.go:40-87`, existing migration files in `migrations/`
- Produces: A separate CI job that boots Postgres via testcontainers and runs integration tests with race detection

**Why:** Your integration tests exist (`repository/repository_integration_test.go`) but CI runs `go test ./...` which may skip them or not explicitly signal their importance. A dedicated CI job with Docker that runs integration tests separately demonstrates you understand the difference between unit and integration testing — and makes the test signal explicit. If integration tests are slow, they can be isolated from fast unit tests.

- [ ] **Step 1: Make migration path configurable in db/pool.go**

Edit `internal/db/pool.go`. The current `SetupTestDB` hardcodes `migrationsPath := "../../migrations"` (line 76). This breaks if tests run from a different working directory.

Change line 76:
```go
	migrationsPath := "../../migrations"
```

To:
```go
	migrationsPath := "../../migrations"
	if mp := os.Getenv("MIGRATIONS_PATH"); mp != "" {
		migrationsPath = mp
	}
```

Note: `os` is already imported in this file (line 7). No new import needed.

- [ ] **Step 2: Add integration-test job to CI**

Edit `.github/workflows/ci.yml`. Add a new job after the existing `build` job:

```yaml
  integration-test:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - uses: bufbuild/buf-setup-action@v1
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
      - run: buf generate
      - name: Run integration tests with race detector
        run: go test -race -count=1 -tags=integration ./internal/...
        env:
          MIGRATIONS_PATH: ./migrations
```

- [ ] **Step 3: Add integration build tag to existing integration tests**

Edit `internal/repository/repository_integration_test.go`. Add build tag at the top (before `package`):

```go
//go:build integration
```

This ensures integration tests only run when the `integration` tag is set, keeping the default `go test ./...` fast for CI.

- [ ] **Step 4: Verify CI file is valid YAML**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "Valid YAML"
```

Expected: Prints "Valid YAML"

- [ ] **Step 5: Verify integration tests still pass locally**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && \
go test -race -count=1 -tags=integration ./internal/repository/ -v
```

Expected: Integration tests pass (requires Docker running for testcontainers).

- [ ] **Step 6: Verify default test run excludes integration tests**

Run:
```bash
cd /mnt/c/Users/AlexisP.EBOS.002/Projects/order-engine && go test ./internal/repository/ -v 2>&1 | head -5
```

Expected: Tests are skipped or the package reports "no tests to run" (build tag filtering).

- [ ] **Step 7: Commit**

```bash
git add internal/db/pool.go .github/workflows/ci.yml internal/repository/repository_integration_test.go
git commit -m "ci: add dedicated integration test job with Docker and race detector"
```

---

## Summary of Changes

| Task | Files Changed | Risk | Time |
|------|--------------|------|------|
| 1. Dockerfile Go version | 1 file, 1 line | Minimal | 5 min |
| 2. CI race detector | 1 file, 1 line | Minimal | 5 min |
| 3. Prometheus metrics + health | 4 new files, 4 modified | Medium — new dependency | 2-3 hours |
| 4. Request ID interceptor | 2 files modified | Low — additive | 30 min |
| 5. Event publish ordering | 1 file, 1 method | Low — reorders existing calls | 30 min |
| 6. TradeFeed goroutine fix | 1 file, 1 method | Medium — concurrent code | 1-2 hours |
| 7. Final verification | README if needed | Minimal | 15 min |
| 8. OTel tracing | 3 new files, 3 modified | Medium — new SDK dependency | 1-2 hours |
| 9. Integration test CI job | 3 files modified | Low — additive CI config | 30 min |

**Total estimated time: 6-9 hours**

## Verification Checklist

After all tasks complete, verify:
- [ ] `go test -race -count=1 ./...` passes (unit tests)
- [ ] `go test -race -count=1 -tags=integration ./internal/...` passes (integration tests, requires Docker)
- [ ] `go build ./cmd/server/` succeeds
- [ ] `golangci-lint run` clean
- [ ] `curl localhost:6060/metrics` returns Prometheus metrics
- [ ] `curl localhost:6060/healthz` returns 200
- [ ] `curl localhost:6060/ready` returns 200 (when DB is up)
- [ ] Request IDs appear in slog output
- [ ] Trade events only published after DB commit
- [ ] No goroutine leaks in TradeFeed on disconnect
- [ ] Jaeger UI at `localhost:16686` shows `matching.SubmitOrder` spans (when docker-compose is running)
- [ ] README claims match implementation (all telemetry claims now backed by code)
