# Order Engine

Real-time order matching engine in Go — gRPC API, in-memory price-time priority book, PostgreSQL persistence, OpenTelemetry + Prometheus observability.

**Key numbers:** ~2.3µs per order submission (with OTel tracing), ~834K ops/s parallel throughput, 27 unit tests + integration + fuzz, ~15MB Docker image.

---

## Architecture

```
                      gRPC clients
                           │
              ┌────────────┴────────────┐
              │     Order Engine (Go)    │
              │                          │
              │  gRPC ←→ MatchingEngine  │
              │  Handlers   (in-memory)  │
              │             (price-time) │
              │       ↓         ↓       │
              │  Event Bus   Repository  │
              │  (pub/sub)   (sqlc/pgx)  │
              └────────┬────────┬────────┘
                       │        │
                  OTel/Prom   PostgreSQL 16
```

## Why this exists

Trading systems need sub-millisecond order matching — DB round-trips add 1-10ms. This engine keeps the hot path entirely in-memory, uses PostgreSQL as the durable system of record, and streams results to clients via gRPC.

## Stack

| Layer | Choice |
|---|---|
| **Language** | Go 1.26 |
| **RPC** | gRPC (unary + server/client/bidi streaming) |
| **Proto** | buf (lint + breaking-change detection) |
| **DB** | PostgreSQL 16 + sqlc (type-safe queries) |
| **Observability** | OpenTelemetry + Prometheus + slog |
| **Matching** | Price-time priority, FIFO within price level |
| **CI** | GitHub Actions (lint → test → integration → build) |

## Key design decisions

- **Sorted slice of price levels** — O(log m) price lookup via binary search, O(m) insertion (m is typically 10-50 levels). Cache-friendly, no GC pressure.
- **Event publish after DB commit** — no phantom events on rollback.
- **Self-trade prevention** — MiFID II compliance in `fillOrdersAtLevel`.
- **Market slippage protection** — `max_slippage_bps` caps fills.
- **Idempotent creation** — `idempotency_key` prevents duplicates.

## Benchmarks

All with OTel tracing active. Hardware: Intel Core Ultra 7 255H.

| Benchmark | Latency | Throughput |
|-----------|---------|------------|
| SubmitOrder (no match) | 2.3µs | 431K ops/s |
| SubmitOrderParallel | 1.2µs | 834K ops/s |
| MatchingWithTrades | 2.0µs | 497K ops/s |
| SubmitOrderLimitRest (1000-book) | 3.9µs | 257K ops/s |
| SubmitOrderLimitMatch (1000-book) | 91µs | 11K ops/s |
| OrderBookSnapshot (100-level) | 10µs | 100K reads/s |

Without OTel: ~0.8µs per order (measured on AMD Ryzen 7 9700X).

## Quick start

```bash
git clone https://github.com/AlexPips/order-engine.git && cd order-engine
make proto    # buf lint + generate
make sqlc     # sqlc generate
docker compose -f deployments/docker-compose.yml up --build
```

```bash
# Submit a limit order
grpcurl -plaintext -d '{
  "user_id": "alice", "symbol": "BTCUSD",
  "side": "SIDE_BUY", "type": "ORDER_TYPE_LIMIT",
  "price": {"value": 500000000000, "precision": 8},
  "quantity": {"value": 50000000, "precision": 8},
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
}' localhost:50051 order.v1.OrderService/CreateOrder
```

## What's included

- 4 gRPC API patterns (unary, server-stream, client-stream, bidi)
- 4 RPC patterns: `CreateOrder`, `GetOrder`, `CancelOrder`, `GetOrderBook`
- Streaming: `StreamOrderUpdates`, `BatchCreateOrders`, `TradeFeed`
- Partial fills, self-trade prevention, market slippage protection
- Prometheus metrics (orders, trades, latency histogram, book depth)
- OTel traces (gRPC → engine → repository, OTLP gRPC export)
- Graceful shutdown (drain RPCs → flush spans → close DB)
- Health checks (`/healthz`, `/ready`)
- State recovery on restart (replay open orders from DB)
- mTLS-ready
- Fuzz tests + integration tests (testcontainers-go)

## Tech reference

Full technical documentation, code walkthroughs, and known limitations:
**[docs/technical-reference.md](docs/technical-reference.md)**

## License

MIT
