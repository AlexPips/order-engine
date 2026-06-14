# Order Engine

A production-grade Go gRPC order management service with an in-memory price-time priority matching engine, PostgreSQL persistence, and full observability.

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

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.23+ | First-class concurrency, fast compile, single static binary |
| RPC | `google.golang.org/grpc` | HTTP/2 + protobuf, native streaming, strong Go ecosystem |
| Proto | `buf` for lint + codegen | Schema linting, breaking-change detection, single-tool pipeline |
| Database | PostgreSQL 16 | ACID transactions, mature replication, native `NUMERIC` for exact decimals |
| Query gen | `sqlc` | Compile-time type checking, no runtime ORM overhead, plain SQL |
| Migrations | `golang-migrate/migrate` | CLI + library, idempotent, integrates with CI |
| Observability | OpenTelemetry + Prometheus | Vendor-neutral tracing, de-facto metrics standard |
| Logging | `log/slog` (stdlib) | Structured logging in stdlib since 1.21, JSON output |
| Config | env vars + `envconfig` | 12-factor, no config parsing on hot path |
| Testing | stdlib `testing` + `testcontainers-go` | Real Postgres in CI, no mock drift |
| Lint | `golangci-lint` | Aggregator for the standard Go linters |
| CI | GitHub Actions | Native to the repo host |

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

Benchmark suite ships with the matching engine (`make bench`). Profiling via `go tool pprof`.

### Results (AMD Ryzen 7 9700X)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| SubmitOrder (no match) | ~800 | ~1,250 | 13 |
| SubmitOrderParallel | ~570 | ~575 | 14 |
| MatchingWithTrades | ~860 | ~1,240 | 13 |
| ConcurrentSubmit | ~920 | ~1,230 | 13 |
| OrderBookSnapshot (100 levels) | ~8,100 | ~15,300 | 501 |

**Target: <3μs per order submission** — achieved ~0.8μs (single-threaded), ~0.57μs (parallel).

## Roadmap

- [x] Repo scaffold + README
- [x] Proto definitions (`order.proto` with all 4 RPC patterns)
- [x] `buf` build config + generation pipeline
- [x] sqlc schema + queries
- [x] Migrations (orders, trades, audit log)
- [x] Domain types (Order, Trade, OrderBook)
- [x] Matching engine (price-time priority, partial fills)
- [x] gRPC server + handlers
- [x] Server-stream event bus (order state + trade events)
- [x] OpenTelemetry + Prometheus setup
- [x] Graceful shutdown
- [x] Dockerfile + docker-compose
- [x] Testcontainers integration tests
- [x] GitHub Actions CI (lint, test, build, proto lint, buf breaking)
- [x] Benchmark suite + pprof examples
- [x] Example client (Go + grpcurl recipes)
- [x] mTLS configuration example
- [x] Final polish pass + architecture diagram commit

## License

MIT
