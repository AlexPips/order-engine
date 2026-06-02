# Order Engine

A production-grade Go gRPC order management service with an in-memory price-time priority matching engine, PostgreSQL persistence, and full observability.

> Demonstrates: protobuf, all 4 gRPC streaming patterns, sqlc-generated repository layer, OpenTelemetry tracing, Prometheus metrics, structured concurrency, graceful shutdown, testcontainers integration tests, and CI.

---

## Why

Built as a portfolio piece targeting backend roles in fintech and trading infrastructure (payment processing, exchange backends, brokerage platforms). The order matching engine and streaming RPCs directly mirror the architecture of real-time trading systems, but the patterns transfer to any high-throughput domain.

The goal is not to compete with a real exchange — it's to ship a small, complete, **honest** service that a senior engineer can read in 15 minutes and immediately trust.

## Architecture

```
                         ┌────────────────────┐
                         │   gRPC clients     │
                         │  (grpcurl, apps)   │
                         └─────────┬──────────┘
                                   │ TLS + mTLS
                                   ▼
   ┌─────────────────────────────────────────────────────────┐
   │                  Order Engine (Go)                      │
   │                                                         │
   │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
   │  │   gRPC       │  │  Matching    │  │  Repository  │    │
   │  │   Handlers   │◄─┤  Engine      │  │  (sqlc)      │    │
   │  │              │  │ (price-time) │  │              │    │
   │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
   │         │                 │                 │            │
   │         │      ┌──────────┴──────────┐      │            │
   │         │      │ Event Bus (chan)    │      │            │
   │         │      │  + OTel spans      │      │            │
   │         │      └──────────┬──────────┘      │            │
   │         │                 │                 │            │
   └─────────┼─────────────────┼─────────────────┼────────────┘
             │                 │                 │
             ▼                 ▼                 ▼
      ┌───────────┐      ┌───────────┐     ┌─────────────┐
      │ OTel      │      │Prometheus │     │ PostgreSQL  │
      │ Collector │      │ /metrics  │     │ (orders,    │
      └───────────┘      └───────────┘     │  trades)    │
                                          └─────────────┘
```

## Features

### gRPC API
- **Unary:** `GetOrder`, `CancelOrder`, `GetOrderBook`
- **Server-streaming:** `StreamOrderUpdates` (real-time order state changes)
- **Client-streaming:** `BatchCreateOrders` (bulk order ingestion)
- **Bidirectional-streaming:** `TradeFeed` (subscribe + inject orders in one stream)

### Matching Engine
- Price-time priority (FIFO within price level)
- Limit and market orders
- Partial fills
- Trade event emission to subscribers
- Thread-safe for concurrent submission (sync.RWMutex on the book, atomic counters)

### Persistence
- PostgreSQL with `sqlc` for type-safe queries
- `golang-migrate` for schema versioning
- Repository pattern — no SQL leaks past `internal/repository/`

### Observability
- OpenTelemetry traces across gRPC handlers → matching engine → repository
- Prometheus metrics: orders submitted, trades executed, order book depth, latency histograms
- Structured logging via `slog` (JSON output)

### Operational
- Graceful shutdown on SIGINT/SIGTERM — drain in-flight RPCs, flush spans, close DB pool
- Health check (`grpc.health.v1.Health`) + readiness
- mTLS-ready (configurable; insecure default for local dev)
- Docker + docker-compose for one-command spin-up

## Tech Stack

| Layer | Choice | Why |
|---|---|---|
| Language | Go 1.23+ | Primary CV language |
| RPC | `google.golang.org/grpc` | Standard |
| Proto | `buf` for lint + generation | Industry standard, schema enforcement |
| Database | PostgreSQL 16 | What fintech shops use |
| Query gen | `sqlc` | Type-safe, compiles, no runtime ORM overhead |
| Migrations | `golang-migrate/migrate` | CLI + library, works in CI |
| Observability | OpenTelemetry + Prometheus | What every target job asks about |
| Logging | `log/slog` (stdlib) | No vendor lock-in |
| Config | env vars + `envconfig` | 12-factor |
| Testing | stdlib `testing` + `testcontainers-go` | Real Postgres in tests, no mocks |
| Lint | `golangci-lint` | Standard |
| CI | GitHub Actions | Matches the repo host |

## Project Structure

```
order-engine/
├── proto/order/v1/             # Protobuf definitions
├── cmd/server/                 # main.go — entry point
├── internal/
│   ├── server/                 # gRPC handler implementations
│   ├── matching/               # Order matching engine (in-memory)
│   ├── repository/             # sqlc-generated queries + interfaces
│   ├── domain/                 # Order, Trade, OrderBook types
│   ├── telemetry/              # OTel + Prometheus setup
│   └── config/                 # env loading + validation
├── migrations/                 # *.up.sql / *.down.sql
├── gen/proto/                  # Generated Go (gitignored)
├── deployments/
│   ├── Dockerfile
│   └── docker-compose.yml
├── .github/workflows/ci.yml
├── buf.gen.yaml
├── sqlc.yaml
├── Makefile
├── go.mod
└── README.md
```

## Quick Start

### Prerequisites
- Go 1.23+
- Docker + docker-compose
- `buf` (proto tooling) — `go install github.com/bufbuild/buf/cmd/buf@latest`
- `protoc-gen-go` + `protoc-gen-go-grpc` — `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`
- `sqlc` — `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- `migrate` — `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

### Run locally

```bash
# 1. Clone & install
git clone https://github.com/AlexPips/order-engine.git
cd order-engine
go mod download

# 2. Generate proto + sqlc code
make proto
make sqlc

# 3. Start Postgres + service
docker compose -f deployments/docker-compose.yml up --build

# 4. Submit a test order (in another terminal)
grpcurl -plaintext -d '{
  "symbol": "BTCUSD",
  "side": "BUY",
  "type": "LIMIT",
  "price": "50000.00",
  "quantity": "0.5"
}' localhost:50051 order.v1.OrderService/CreateOrder
```

## Development

```bash
make help           # List all targets
make proto          # Regenerate protobuf Go code
make sqlc           # Regenerate sqlc queries
make test           # Unit + integration tests (requires Docker)
make bench          # Run matching engine benchmarks
make lint           # golangci-lint run
make migrate-up     # Apply DB migrations
make migrate-down   # Roll back last migration
```

## Example RPCs

```bash
# Stream live order book updates
grpcurl -plaintext -d '{"symbol":"BTCUSD"}' \
  localhost:50051 order.v1.OrderService/StreamOrderUpdates

# Submit a batch of orders
grpcurl -plaintext -d @ localhost:50051 order.v1.OrderService/BatchCreateOrders <<EOF
{"orders":[
  {"symbol":"BTCUSD","side":"BUY","type":"LIMIT","price":"50000","quantity":"0.1"},
  {"symbol":"BTCUSD","side":"BUY","type":"LIMIT","price":"49900","quantity":"0.2"}
]}
EOF
```

## Benchmarks

Matching engine throughput, measured on Apple M2 / Go 1.23:

| Scenario | Throughput | p99 Latency |
|---|---|---|
| Single-symbol, 10 concurrent submitters | TBD | TBD |
| Multi-symbol (100 symbols), 100 concurrent | TBD | TBD |
| Order book depth 10k, market order match | TBD | TBD |

Run with `make bench`. Profiling via `go tool pprof`.

## Roadmap

- [x] Repo scaffold + README
- [ ] Proto definitions (`order.proto` with all 4 RPC patterns)
- [ ] `buf` build config + generation pipeline
- [ ] sqlc schema + queries
- [ ] Migrations (orders, trades, audit log)
- [ ] Domain types (Order, Trade, OrderBook)
- [ ] Matching engine (price-time priority, partial fills)
- [ ] gRPC server + handlers
- [ ] Server-stream event bus (order state + trade events)
- [ ] OpenTelemetry + Prometheus setup
- [ ] Graceful shutdown
- [ ] Dockerfile + docker-compose
- [ ] Testcontainers integration tests
- [ ] GitHub Actions CI (lint, test, build, proto lint, buf breaking)
- [ ] Benchmark suite + pprof examples
- [ ] Example client (Go + grpcurl recipes)
- [ ] mTLS configuration example
- [ ] Final polish pass + architecture diagram commit

## License

MIT
