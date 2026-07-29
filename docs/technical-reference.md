# Order Engine — Technical Reference

> A production-grade Go gRPC order management service with an in-memory price-time priority matching engine, PostgreSQL persistence, and full observability (Prometheus + OpenTelemetry).

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Tech Stack](#2-tech-stack)
3. [Project Structure](#3-project-structure)
4. [Architecture](#4-architecture)
5. [Domain Model](#5-domain-model)
6. [gRPC API](#6-grpc-api)
7. [Matching Engine](#7-matching-engine)
8. [Persistence Layer](#8-persistence-layer)
9. [Event Bus](#9-event-bus)
10. [Request Pipeline (Interceptors)](#10-request-pipeline-interceptors)
11. [Observability](#11-observability)
12. [Configuration](#12-configuration)
13. [Deployment](#13-deployment)
14. [CI/CD](#14-cicd)
15. [Development Guide](#15-development-guide)
16. [Testing Strategy](#16-testing-strategy)
17. [Production Fixes (Branch)](#17-production-fixes-branch)

---

## 1. Project Overview

Order Engine is a real-time order management and matching system built in Go. It accepts orders via gRPC, executes them against an in-memory order book using price-time priority, persists results to PostgreSQL, and streams trade/order-update events to connected clients.

**Key capabilities:**
- 4 gRPC API patterns: unary, server-streaming, client-streaming, bidirectional-streaming
- In-memory limit/market order matching with partial fills
- Self-trade prevention (STP) — MiFID II compliant
- Market order slippage protection via `max_slippage_bps`
- Idempotent order creation via `idempotency_key`
- State recovery on restart (replays open orders from DB into memory)
- Prometheus metrics + OpenTelemetry traces + structured logging

---

## 2. Tech Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.26+ | First-class concurrency, fast compile, single static binary |
| RPC | `google.golang.org/grpc` (v1.81) | HTTP/2 + protobuf, native streaming, strong Go ecosystem |
| Proto | `buf` for lint + codegen | Schema linting, breaking-change detection, single-tool pipeline |
| Database | PostgreSQL 16 | ACID transactions, mature replication, native `NUMERIC` for exact decimals |
| Query gen | `sqlc` (v1.31) | Compile-time type checking, no runtime ORM overhead, plain SQL |
| Migrations | `golang-migrate/migrate` | CLI + library, idempotent, integrates with CI |
| Observability | Prometheus + OpenTelemetry | Vendor-neutral tracing, de-facto metrics standard |
| Logging | `log/slog` (stdlib) | Structured logging in stdlib since 1.21, JSON output |
| Config | env vars (no library) | 12-factor, no config parsing on hot path |
| Testing | stdlib `testing` + `testcontainers-go` | Real Postgres in CI, no mock drift |
| Lint | `golangci-lint` v2.12 | Aggregator for the standard Go linters |
| CI | GitHub Actions | Native to the repo host |
| Decimal | `shopspring/decimal` | Exact arithmetic for financial quantities |
| IDs | `google/uuid` | Unique order/trade/user identifiers |

---

## 3. Project Structure

```
order-engine/
├── proto/order/v1/
│   └── order.proto              # Protobuf service + message definitions
├── gen/proto/                   # Generated Go protobuf code (gitignored)
├── cmd/server/
│   └── main.go                  # Entry point: wiring, startup, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go            # Env-var loading (DATABASE_URL, GRPC_PORT)
│   ├── domain/
│   │   ├── id.go                # OrderID, TradeID, UserID (typed strings)
│   │   ├── order.go             # Order struct, Side/OrderType/OrderStatus enums
│   │   └── trade.go             # Trade struct
│   ├── matching/
│   │   ├── engine.go            # Engine: SubmitOrder entry point, matchLimit/matchMarket
│   │   ├── types.go             # OrderBook, PriceLevel, OrderBookSnapshot
│   │   ├── queue.go             # OrderBook.insertOrder, snapshot, prune, bestBid/bestAsk
│   │   ├── engine_test.go       # Unit tests for matching logic
│   │   ├── bench_test.go        # Quick benchmarks
│   │   ├── benchmark_test.go    # Comprehensive benchmarks
│   │   └── fuzz_test.go         # Fuzz testing for matching engine
│   ├── server/
│   │   └── order_service.go     # gRPC handler implementations (all 7 RPCs)
│   ├── events/
│   │   ├── bus.go               # In-memory pub/sub event bus
│   │   └── bus_test.go          # Event bus unit tests
│   ├── repository/
│   │   ├── db.go                # sqlc-generated DB interface
│   │   ├── models.go            # sqlc-generated DB row types
│   │   ├── orders.sql.go        # sqlc-generated order queries
│   │   ├── trades.sql.go        # sqlc-generated trade queries
│   │   └── repository_integration_test.go  # Integration tests (build tag: integration)
│   ├── db/
│   │   └── pool.go              # pgxpool connection + SetupTestDB (testcontainers)
│   ├── telemetry/
│   │   ├── metrics.go           # Prometheus metric definitions
│   │   ├── metrics_test.go      # Metrics unit tests
│   │   ├── health.go            # Liveness + readiness HTTP handlers
│   │   ├── tracing.go           # OpenTelemetry TracerProvider init (OTLP gRPC)
│   │   └── tracing_test.go      # Tracing tests
│   └── interceptors/
│       └── interceptors.go      # gRPC interceptors: recovery, request ID, logging
├── migrations/
│   ├── 000001_create_orders.up.sql / .down.sql
│   └── 000002_create_trades.up.sql / .down.sql
├── deployments/
│   ├── Dockerfile               # Multi-stage build (golang:1.26-alpine → alpine:3.20)
│   └── docker-compose.yml       # Postgres + server stack
├── .github/workflows/
│   └── ci.yml                   # Lint → Test → Integration → Build
├── sqlc.yaml                    # sqlc configuration
├── buf.gen.yaml                 # buf code generation config
├── buf.yaml                     # buf module config + lint rules
├── .golangci.yml                # golangci-lint v2 configuration
├── Makefile                     # Build/test/lint/proto/docker targets
├── go.mod / go.sum              # Go module dependencies
└── docs/
    ├── technical-reference.md   # This document
    └── superpowers/
        ├── plans/
        │   └── 2026-07-28-production-fixes.md  # Implementation plan
        └── specs/
            └── ...                              # Design specs
```

---

## 4. Architecture

### Component Diagram

```
                         ┌──────────────────────┐
                         │    gRPC Clients       │
                         │ (grpcurl, apps, SDK)  │
                         └───────────┬──────────┘
                                     │
                                     ▼
         ┌─────────────────────────────────────────────────────┐
         │                  Order Engine (Go)                  │
         │                                                     │
         │  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
         │  │   gRPC       │  │  Matching    │  │ Repository│ │
         │  │   Handlers   │◄─┤  Engine      │  │ (sqlc)    │ │
         │  │ (OrderService) │  │ (in-memory) │  │           │ │
         │  └──────┬───────┘  └──────┬───────┘  └─────┬─────┘ │
         │         │                 │                 │       │
         │         │      ┌──────────┴──────────┐      │       │
         │         │      │   Event Bus (chan)  │      │       │
         │         │      │  (pub/sub in-memory)│      │       │
         │         │      └──────────┬──────────┘      │       │
         │         │                 │                 │       │
         └─────────┼─────────────────┼─────────────────┼───────┘
                   │                 │                 │
                   ▼                 ▼                 ▼
            ┌───────────┐     ┌───────────┐     ┌─────────────┐
            │   OTel    │     │ Prometheus│     │ PostgreSQL  │
            │ Collector │     │ /metrics  │     │ (orders,    │
            │  (traces) │     │ :6060     │     │  trades)    │
            └───────────┘     └───────────┘     └─────────────┘
```

### Request Flow (CreateOrder example)

```
Client                   gRPC Server                    Matching Engine         PostgreSQL
  │                          │                              │                     │
  │── CreateOrder ──────────►│                              │                     │
  │                          │                              │                     │
  │                    ┌─────┴─────┐                        │                     │
  │                    │ Interceptors                       │                     │
  │                    │ 1. Recovery (panic→500)            │                     │
  │                    │ 2. RequestID (inject UUID)         │                     │
  │                    │ 3. Logging (method+code+duration)  │                     │
  │                    │ 4. OTel StatsHandler (trace span)  │                     │
  │                    └─────┬─────┘                        │                     │
  │                          │                              │                     │
  │                    ┌─────┴─────┐                        │                     │
  │                    │ CreateOrder handler                │                     │
  │                    │ • Idempotency check                │                     │
  │                    │ • Convert proto → domain           │                     │
  │                    │ • Inc OrdersReceived counter       │                     │
  │                    │•─ SubmitOrder ──────────────────►  │                     │
  │                    │   │              ┌────────────┐    │                     │
  │                    │   │              │ OTel span  │    │                     │
  │                    │   │              │ attributes │    │                     │
  │                    │   │              └────────────┘    │                     │
  │                    │   │◄── trades ───────────────────  │                     │
  │                    │ • Observe OrderLatency histogram   │                     │
  │                    │ • Inc OrdersSubmitted counter      │                     │
  │                    │ • Inc TradesExecuted counter       │                     │
  │                    │•─ persistOrderTx ───────────────────────────────► BEGIN  │
  │                    │   │                                              │      │
  │                    │   │── CreateOrder ───────────────────────────────►│      │
  │                    │   │── CreateTrade (×N) ──────────────────────────►│      │
  │                    │   │                                              │      │
  │                    │   │◄── COMMIT ────────────────────────────────────│      │
  │                    │   │                                              │      │
  │                    │   │── Publish trade events ──► Event Bus ────────►│      │
  │                    │                                                │         │
  │◄── Order ──────────┤◄────────────────────────────────────────────────│         │
  │                    │                                                │         │
```

### Graceful Shutdown Sequence

```
SIGINT/SIGTERM
    │
    ├─► grpcServer.GracefulStop()    — drain in-flight RPCs (stops accepting new)
    ├─► metricsServer.Close()        — stop HTTP metrics server
    ├─► pool.Close()                 — close PostgreSQL connection pool
    └─► tp.Shutdown(ctx)             — flush remaining OTel spans to collector
```

---

## 5. Domain Model

### Types

```go
type OrderID string    // UUID
type TradeID string    // UUID
type UserID string     // arbitrary string from client
```

### Enums

```go
type Side int
const (
    SideBuy  Side = 0  // "BUY"
    SideSell Side = 1  // "SELL"
)

type OrderType int
const (
    OrderTypeLimit  OrderType = 0  // "LIMIT"
    OrderTypeMarket OrderType = 1  // "MARKET"
)

type OrderStatus int
const (
    OrderStatusNew      OrderStatus = 0  // "NEW"
    OrderStatusPartial  OrderStatus = 1  // "PARTIAL"
    OrderStatusFilled   OrderStatus = 2  // "FILLED"
    OrderStatusCanceled OrderStatus = 3  // "CANCELED"
    OrderStatusRejected OrderStatus = 4  // "REJECTED"
)
```

All enums have `String()` methods returning the string representations above.

### Order

```go
type Order struct {
    ID             OrderID
    UserID         UserID
    Symbol         string          // e.g. "BTCUSD"
    Side           Side
    Type           OrderType
    Price          decimal.Decimal // limit price (decimal with 8 places)
    Quantity       decimal.Decimal
    FilledQty      decimal.Decimal
    Status         OrderStatus
    CreatedAt      int64           // unix nano
    UpdatedAt      int64           // unix nano
    MaxSlippageBPS uint32          // market order only, caps fill price
}
```

### Trade

```go
type Trade struct {
    ID          TradeID
    Symbol      string
    BuyOrderID  OrderID
    SellOrderID OrderID
    Price       decimal.Decimal
    Quantity    decimal.Decimal
    ExecutedAt  int64             // unix nano
}
```

---

## 6. gRPC API

### Service Definition

```protobuf
service OrderService {
    // Unary
    rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse);
    rpc GetOrder(GetOrderRequest) returns (GetOrderResponse);
    rpc CancelOrder(CancelOrderRequest) returns (CancelOrderResponse);
    rpc GetOrderBook(GetOrderBookRequest) returns (GetOrderBookResponse);

    // Server-streaming: client subscribes, server pushes order updates
    rpc StreamOrderUpdates(StreamOrderUpdatesRequest) returns (stream StreamOrderUpdatesResponse);

    // Client-streaming: client sends batch, server returns aggregated response
    rpc BatchCreateOrders(stream BatchCreateOrdersRequest) returns (BatchCreateOrdersResponse);

    // Bidirectional-streaming: subscribe to trades + inject orders in one stream
    rpc TradeFeed(stream TradeFeedRequest) returns (stream TradeFeedResponse);
}
```

### Decimal Wire Format

Prices and quantities use a custom `Decimal` type to avoid floating-point precision issues:

```protobuf
message Decimal {
    int64 value = 1;      // unscaled coefficient
    int32 precision = 2;  // negative exponent (e.g., precision=2 → scale by 10^-2)
}
```

Example: `12.34` → `{value: 1234, precision: 2}`

### RPC Details

#### 4.1 CreateOrder (Unary)

**Request:**
```json
{
  "user_id": "alice",
  "symbol": "BTCUSD",
  "side": "SIDE_BUY",
  "type": "ORDER_TYPE_LIMIT",
  "price": {"value": 500000000000, "precision": 8},
  "quantity": {"value": 50000000, "precision": 8},
  "idempotency_key": "uuid-here",
  "max_slippage_bps": 0
}
```

**Behavior:**
- `idempotency_key` → used as the order ID. Duplicate keys return `AlreadyExists`.
- `max_slippage_bps` — for market orders, caps fills at `bestPrice × (1 ± bps/10000)`. 0 = no limit.
- Writes order + trades in a single PostgreSQL transaction.
- Publishes trade events to Event Bus AFTER commit (no phantom events on rollback).

**Response:** `{ "order": { ... } }` with full order state.

---

#### 4.2 GetOrder (Unary)

**Request:** `{ "id": "order-uuid" }`

Looks up order by ID in PostgreSQL. Returns `NotFound` if missing.

---

#### 4.3 CancelOrder (Unary)

**Request:** `{ "id": "order-uuid" }`

Sets `status = CANCELED` in PostgreSQL. Updates in-memory order state. Publishes `order.cancel` event. Returns `NotFound` if order doesn't exist.

---

#### 4.4 GetOrderBook (Unary)

**Request:** `{ "symbol": "BTCUSD" }`

Returns snapshot of the in-memory order book: bids (sorted high→low) and asks (sorted low→high), each with price, total quantity, and order count.

---

#### 4.5 StreamOrderUpdates (Server-streaming)

**Request:** `{ "symbol": "BTCUSD" }` or `{ "user_id": "alice" }`

Opens a long-lived stream. Subscribes to the Event Bus `order.update` topic. Sends the full order proto when an update is received. Supports optional filtering by symbol or user ID (via oneof). Stream stays open until client disconnects.

---

#### 4.6 BatchCreateOrders (Client-streaming)

**Request stream:** sequence of `{ order: { ...CreateOrderRequest } }`

Accepts multiple orders on one stream. Each order is independently validated and executed (idempotency check → engine → persist). Duplicates are silently skipped. On client EOF, returns list of all successfully created orders.

---

#### 4.7 TradeFeed (Bidirectional-streaming)

**Request stream messages:**
- `subscribe_symbol: "BTCUSD"` — subscribe to trades for a symbol (spawns a sender goroutine)
- `new_order: { ...CreateOrderRequest }` — inject and execute an order

**Response stream messages:**
- `trade: { ...Trade }` — filled trade event for subscribed symbol

Each subscription spawns a goroutine that listens on the Event Bus channel and sends trades to the client. Goroutines clean up on context cancellation. Clients can subscribe to multiple symbols simultaneously.

**Goroutine lifecycle:**
1. Client sends `subscribe_symbol`
2. Handler creates `chan any` on Event Bus, spawns sender goroutine
3. Sender goroutine loops: `select { case msg ← tradeCh: send to stream; case ← ctx.Done(): return }`
4. On client disconnect, all channels are unsubscribed and `wg.Wait()` ensures senders exit before the method returns

---

## 7. Matching Engine

### Algorithm

**Price-time priority (FIFO within price level):**

1. Incoming order checks against opposite side of the book.
2. For each price level, orders are filled in FIFO order (oldest first).
3. If the incoming order is fully filled, it's done.
4. If the incoming order has remaining quantity, it's inserted as a resting order on its own side.
5. Self-trade prevention: orders with the same `UserID` are skipped during matching (no fill against self).

### Price Levels

```go
type PriceLevel struct {
    Price  decimal.Decimal
    Orders []domain.Order   // FIFO queue at this price
}

type OrderBook struct {
    mu     sync.RWMutex
    bids   []PriceLevel          // sorted high→low (buyers)
    asks   []PriceLevel          // sorted low→high (sellers)
    orders map[OrderID]*Order    // O(1) order lookup for cancelation
}
```

Bids are sorted descending (highest price first), asks ascending (lowest price first). Each price level contains a FIFO queue of orders at that price. The entire book is a sorted slice of `PriceLevel` values.

**Complexity:**
- Price level lookup: O(log m) — binary search via `sort.Search`, where m = number of distinct price levels
- Price level insertion: O(m) — inserting into a sorted slice requires shifting elements. In practice m is small (typically 10–50 levels per symbol)
- Order lookup by ID: O(1) — `map[OrderID]*Order`
- Matching against opposite side: O(k·p) — iterates k matching levels, each with p orders

**Why a sorted slice instead of a balanced tree?** For a single-symbol order book, the number of price levels rarely exceeds 100. The simplicity of a sorted slice (cache-friendly iteration, zero pointer chasing, no GC pressure) outweighs the theoretical O(m) insertion cost. At scale (1000+ symbols), sharding by symbol is the right approach — not a per-book tree.

### Limit Order Matching

```
matchLimit(ctx, book, incoming, priceLimit):
  for each level on opposite side:
    if level.price breaks priceLimit: break
    fill orders at this level FIFO
    prune empty levels
  if remaining > 0: insert as resting order
```

### Market Order Matching

```
matchMarket(ctx, book, incoming):
  if max_slippage_bps > 0:
    compute priceLimit = bestPrice × (1 ± bps/10000)
  else:
    set artificial price limit (very high for buys, zero for sells)
  delegate to matchLimit(ctx, book, incoming, priceLimit)
```

### Self-Trade Prevention (STP)

In `fillOrdersAtLevel`, orders from the same `UserID` are skipped:

```go
if incoming.UserID == ro.UserID {
    j++
    continue
}
```

This means an institution cannot trade with itself through the engine (MiFID II requirement). The order remains in the book and may match against other participants.

### Book Depth Gauge

A background goroutine in `main.go` updates the `order_book_depth` Prometheus gauge every 5 seconds by iterating over all symbols and measuring `len(bids) + len(asks)`.

### Performance Benchmarks

All benchmarks run with OTel tracing active (matches production configuration). Hardware: Intel Core Ultra 7 255H, Go 1.26.

| Benchmark | ns/op | B/op | allocs/op | Est. throughput |
|-----------|-------|------|-----------|-----------------|
| SubmitOrder (no match) | 2,321 | ~1,250 | 13 | ~431K ops/s |
| SubmitOrderParallel | 1,199 | ~575 | 14 | ~834K ops/s |
| MatchingWithTrades | 2,010 | ~1,240 | 13 | ~497K ops/s |
| ConcurrentSubmit | 2,239 | ~1,230 | 13 | ~447K ops/s |
| SubmitOrderLimitRest (1000-book) | 3,888 | — | — | ~257K ops/s |
| SubmitOrderLimitMatch (1000-book) | 90,929 | — | — | ~11K ops/s |
| SubmitOrderMarket (1000-book) | 86,017 | — | — | ~12K ops/s |
| OrderBookSnapshot (100 levels) | 10,007 | ~15,300 | 501 | ~100K reads/s |
| GetOrderBook (1000-book) | 95,142 | — | — | ~10K reads/s |

**Notes:**
- "No match" benchmarks insert a limit order that does not cross the spread.
- "1000-book" benchmarks run against a prepopulated book with 1000 resting orders across 100 price levels.
- SubmitOrderLimitMatch crosses the spread and matches against 50+ levels — the higher latency reflects the matching loop, not the data structure.
- Mean ns/op ≈ median (p50) for this workload because the matching engine is single-threaded per book with no contended lock paths.
- Throughput is calculated as `1s / (ns/op × 10⁻⁹)`. Real end-to-end throughput including gRPC deserialization, interceptors, DB writes, and event bus fan-out will be lower.

**Target: <3µs per simple order submission** — achieved (2.3µs with OTel tracing, 1.2µs parallel).

**Comparison:** Without OTel tracing, the same benchmarks run ~0.8µs per order (measured on AMD Ryzen 7 9700X, original development machine). The OTel span creation overhead accounts for the difference.

### 6.8 Example gRPC Calls (grpcurl)

These examples assume a running server on `localhost:50051` with `-plaintext` (insecure mode). For the decimal wire format, prices use `{value, precision}` as described above.

#### CreateOrder (limit buy)

```bash
grpcurl -plaintext -d '{
  "user_id": "alice",
  "symbol": "BTCUSD",
  "side": "SIDE_BUY",
  "type": "ORDER_TYPE_LIMIT",
  "price": {"value": 500000000000, "precision": 8},
  "quantity": {"value": 50000000, "precision": 8},
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440000"
}' localhost:50051 order.v1.OrderService/CreateOrder
```

**Response:**
```json
{
  "order": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": "alice",
    "symbol": "BTCUSD",
    "side": "SIDE_BUY",
    "type": "ORDER_TYPE_LIMIT",
    "price": {"value": 500000000000, "precision": 8},
    "quantity": {"value": 50000000, "precision": 8},
    "filled_qty": {"value": 0, "precision": 8},
    "status": "ORDER_STATUS_NEW"
  }
}
```

Replaying the same request with the same `idempotency_key` returns `AlreadyExists`.

#### CreateOrder (market buy with slippage protection)

```bash
grpcurl -plaintext -d '{
  "user_id": "bob",
  "symbol": "BTCUSD",
  "side": "SIDE_BUY",
  "type": "ORDER_TYPE_MARKET",
  "quantity": {"value": 100000000, "precision": 8},
  "idempotency_key": "660e8400-e29b-41d4-a716-446655440001",
  "max_slippage_bps": 50
}' localhost:50051 order.v1.OrderService/CreateOrder
```

A market buy with `max_slippage_bps=50` caps the fill price at `bestAsk × 1.005`. If no ask exists, returns `InsufficientLiquidity`.

#### GetOrderBook

```bash
grpcurl -plaintext -d '{"symbol": "BTCUSD"}' \
  localhost:50051 order.v1.OrderService/GetOrderBook
```

**Response:**
```json
{
  "bids": [
    {"price": {"value": 500000000000, "precision": 8}, "quantity": {"value": 50000000, "precision": 8}, "order_count": 1}
  ],
  "asks": [
    {"price": {"value": 501000000000, "precision": 8}, "quantity": {"value": 25000000, "precision": 8}, "order_count": 1}
  ]
}
```

Bids are sorted descending (highest price first), asks ascending.

#### StreamOrderUpdates (server-streaming)

```bash
grpcurl -plaintext -d '{"symbol": "BTCUSD"}' \
  localhost:50051 order.v1.OrderService/StreamOrderUpdates
```

Opens a long-lived stream. Submit orders in another terminal to see updates arrive in real time. Stream stays open until the client interrupts (Ctrl+C).

#### BatchCreateOrders (client-streaming)

```bash
grpcurl -plaintext -d @ localhost:50051 order.v1.OrderService/BatchCreateOrders <<EOF
{"order": {"user_id": "alice", "symbol": "BTCUSD", "side": "SIDE_BUY", "type": "ORDER_TYPE_LIMIT", "price": {"value": 499000000000, "precision": 8}, "quantity": {"value": 10000000, "precision": 8}, "idempotency_key": "770e8400-e29b-41d4-a716-446655440002"}}
{"order": {"user_id": "alice", "symbol": "BTCUSD", "side": "SIDE_SELL", "type": "ORDER_TYPE_LIMIT", "price": {"value": 502000000000, "precision": 8}, "quantity": {"value": 10000000, "precision": 8}, "idempotency_key": "880e8400-e29b-41d4-a716-446655440003"}}
EOF
```

Each JSON line is one message in the request stream. The response aggregates all created orders.

#### TradeFeed (bidirectional-streaming)

```bash
grpcurl -plaintext -d @ localhost:50051 order.v1.OrderService/TradeFeed <<EOF
{"subscribe_symbol": "BTCUSD"}
{"new_order": {"user_id": "alice", "symbol": "BTCUSD", "side": "SIDE_BUY", "type": "ORDER_TYPE_LIMIT", "price": {"value": 500000000000, "precision": 8}, "quantity": {"value": 50000000, "precision": 8}, "idempotency_key": "990e8400-e29b-41d4-a716-446655440004"}}
{"new_order": {"user_id": "bob", "symbol": "BTCUSD", "side": "SIDE_SELL", "type": "ORDER_TYPE_LIMIT", "price": {"value": 500000000000, "precision": 8}, "quantity": {"value": 25000000, "precision": 8}, "idempotency_key": "aa0e8400-e29b-41d4-a716-446655440005"}}
EOF
```

After subscribing, each `new_order` is executed and any resulting trades are streamed back. This lets you watch fills in real time.

---

## 8. Persistence Layer

### Schema

#### Orders Table

```sql
CREATE TABLE orders (
    id         TEXT PRIMARY KEY,               -- UUID from idempotency_key
    user_id    TEXT NOT NULL,
    symbol     TEXT NOT NULL,
    side       TEXT NOT NULL CHECK (side IN ('BUY', 'SELL')),
    type       TEXT NOT NULL CHECK (type IN ('LIMIT', 'MARKET')),
    price      NUMERIC(20,8) NOT NULL,
    quantity   NUMERIC(20,8) NOT NULL,
    filled_qty NUMERIC(20,8) NOT NULL DEFAULT 0,
    status     TEXT NOT NULL CHECK (status IN ('NEW','PARTIAL','FILLED','CANCELED','REJECTED')),
    created_at BIGINT NOT NULL,                -- unix nano
    updated_at BIGINT NOT NULL                 -- unix nano
);
CREATE INDEX idx_orders_symbol_created ON orders (symbol, created_at);
CREATE INDEX idx_orders_user_id ON orders (user_id);
```

#### Trades Table

```sql
CREATE TABLE trades (
    id            TEXT PRIMARY KEY,
    symbol        TEXT NOT NULL,
    buy_order_id  TEXT NOT NULL,
    sell_order_id TEXT NOT NULL,
    price         NUMERIC(20,8) NOT NULL,
    quantity      NUMERIC(20,8) NOT NULL,
    executed_at   BIGINT NOT NULL              -- unix nano
);
CREATE INDEX idx_trades_symbol ON trades (symbol);
CREATE INDEX idx_trades_buy_order ON trades (buy_order_id);
CREATE INDEX idx_trades_sell_order ON trades (sell_order_id);
```

### sqlc Queries

SQL queries are defined in `.sql` files and type-safe Go code is generated by `sqlc`:

- **CreateOrder** — INSERT with RETURNING
- **GetOrder** — SELECT by ID
- **GetAllOpenOrders** — SELECT with status IN ('NEW','PARTIAL')
- **CancelOrder** — UPDATE status + RETURNING
- **CreateTrade** — INSERT with RETURNING

### Repository Pattern

`repository.Queries` wraps `pgxpool.Pool` and provides `WithTx(tx)` for transactional execution. All DB access goes through this layer — no raw SQL outside `internal/repository/`.

### Transaction Guarantees

```
BEGIN
    INSERT order
    INSERT trades (×N)
COMMIT
    → Publish events to Event Bus (ONLY after COMMIT succeeds)
```

This ordering ensures that if the transaction rolls back, no phantom trade events are published.

### State Recovery

On startup, `OrderService.RecoverState()` loads all open orders (NEW, PARTIAL status) from PostgreSQL and replays them into the matching engine via `Engine.ReplayOrders()`. This restores the in-memory order book to its pre-restart state.

---

## 9. Event Bus

### Structure

```go
type Bus struct {
    mu       sync.RWMutex
    handlers map[string][]Handler        // synchronous callbacks
    subs     map[string][]chan any       // asynchronous channels
}
```

### Topics

| Topic Pattern | Published By | Payload |
|---|---|---|
| `trade.<symbol>` | `persistOrderTx` (after commit) | `TradeEvent{Symbol, BuyID, SellID, Price, Qty}` |
| `order.update` | `StreamOrderUpdates` (via goroutine) | `OrderUpdateEvent{OrderID, Symbol, Status}` |
| `order.cancel` | `CancelOrder` handler | `OrderUpdateEvent` |

### Delivery Semantics

- **Handlers** (`Subscribe`): synchronous, called in publish goroutine.
- **Channels** (`SubscribeChannel`): asynchronous with buffered channel + `default` (non-blocking send). If a consumer is slow, messages are dropped.
- **Unsubscription**: `UnsubscribeChannel` removes the channel from the subscribers list and closes it.
- **Thread safety**: `sync.RWMutex` protects all maps. Reads (Publish) use RLock, writes (Subscribe/Unsubscribe) use Lock.

---

## 10. Request Pipeline (Interceptors)

Applied in this order on the gRPC server:

```
grpc.StatsHandler(otelgrpc.NewServerHandler())   // OTel tracing span per request
grpc.ChainUnaryInterceptor(
    RecoveryUnary(),       // recover() panic → codes.Internal
    RequestIDUnary(),      // inject UUID into context, log start/end
    LoggingUnary(),        // log method, status code, duration
)
grpc.ChainStreamInterceptor(
    RecoveryStream(),
    RequestIDStream(),     // wraps ServerStream with request ID in context
    LoggingStream(),
)
```

### Recovery
Catches panics, logs stack trace, returns `codes.Internal`. Prevents a single panicking handler from taking down the process.

### Request ID
Injects a UUID into `context.Context` at the start of each request. Logged at both start and end of the request. Accessible via `interceptors.RequestIDFromContext(ctx)`. For streams, wraps the `grpc.ServerStream` to inject the request ID into the stream's context.

### Logging
Logs `method`, `code`, `duration` for every RPC at `slog.Info` level using the standard library's `log/slog`.

### OTel StatsHandler
Uses `otelgrpc.NewServerHandler()` (otelgrpc v0.69+) as a `grpc.StatsHandler` to create OTel trace spans for every gRPC call. This is the modern API — replaces the deprecated `UnaryServerInterceptor`/`StreamServerInterceptor` pattern.

---

## 11. Observability

### 11.1 Prometheus Metrics

Served on `:6060/metrics` via a separate HTTP server (not on the gRPC port).

| Metric | Type | Labels | Description |
|---|---|---|---|
| `orders_received_total` | Counter | — | Total orders received by the service |
| `orders_submitted_total` | Counter | — | Orders successfully submitted to matching engine |
| `trades_executed_total` | Counter | — | Trades executed by matching engine |
| `order_submit_duration_seconds` | Histogram | — | Latency of `engine.SubmitOrder`, buckets 1µs→~1s |
| `order_book_depth` | Gauge | `symbol` | Price levels per symbol (updated every 5s) |
| `active_trade_streams` | Gauge | — | Active TradeFeed streams |

### 11.2 OpenTelemetry Tracing

- **Exporter**: OTLP gRPC → `otel-collector:4317` (insecure, configurable)
- **Provider**: `BatchSpanProcessor` with 5-second batch interval
- **Propagation**: `TraceContext` + `Baggage` (W3C traceparent)
- **Resource attributes**: `service.name`, `service.version`, `telemetry.sdk.language`
- **Instrumentation**:
  - All gRPC calls via `otelgrpc.NewServerHandler()` stats handler
  - `Engine.SubmitOrder` creates a span with order attributes (id, symbol, side, type, price, quantity)
  - Errors recorded via `span.RecordError()`

### 11.3 Health Checks

| Endpoint | Purpose | Behavior |
|---|---|---|
| `/healthz` | Liveness probe | Always returns 200 "ok" |
| `/ready` | Readiness probe | Pings DB; returns 200 if healthy, 503 if down |

Also available programmatically via `Health.Ready(ctx) bool`.

### 11.4 Structured Logging

Uses `log/slog` with default text handler. Key log events:

| Event | Fields |
|---|---|
| Server start | `addr` |
| gRPC unary request | `method`, `code`, `duration`, `request_id` |
| gRPC stream | `method`, `code`, `duration`, `request_id` |
| Panic recovery | `method`, `panic`, `stack` |
| OTel tracer init | (log level: info on success, warn on failure) |
| Shutdown | — |

---

## 12. Configuration

| Env Var | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `GRPC_PORT` | No | `50051` | gRPC server listen port |

Config is loaded eagerly at startup. Missing `DATABASE_URL` causes immediate exit with a clear error message.

---

## 13. Deployment

### Docker

Multi-stage build (`deployments/Dockerfile`):

```dockerfile
# Stage 1: Build
FROM golang:1.26-alpine AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server/

# Stage 2: Runtime
FROM alpine:3.20
COPY --from=builder /server /server
EXPOSE 50051
ENTRYPOINT ["/server"]
```

Final image is ~15MB (static binary + Alpine + CA certs).

### docker-compose

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
    volumes: pgdata
  server:
    build: .
    ports: 50051:50051
    environment: DATABASE_URL, GRPC_PORT
    depends_on: postgres
```

Start: `docker compose -f deployments/docker-compose.yml up --build`

---

## 14. CI/CD

GitHub Actions workflow (`.github/workflows/ci.yml`) runs on push/PR to `main`:

```
lint ──► test ──► integration ──► build
  │        │          │              │
  │        │          │              └── go build ./cmd/server/
  │        │          └── go test -tags=integration -race ./internal/repository/...
  │        └── go test -race -coverprofile=coverage.out ./...
  └── buf generate + golangci-lint run
```

Jobs run sequentially: integration and build wait for test to pass. This prevents wasting CI minutes on Docker-heavy integration tests if unit tests fail.

---

## 15. Development Guide

### Prerequisites

- Go 1.26+
- Docker + docker-compose
- `buf` — `go install github.com/bufbuild/buf/cmd/buf@latest`
- `protoc-gen-go` + `protoc-gen-go-grpc` — `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`
- `sqlc` — `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- `golang-migrate` — `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

### Common Commands

```bash
make build              # Compile server binary
make test               # Run all unit tests (27 tests as of latest)
make test-integration   # Run integration tests (build tag: integration, requires Docker)
make bench              # Run matching engine benchmarks
make lint               # golangci-lint run
make proto              # buf lint + buf generate
make sqlc               # sqlc generate
make docker-up          # docker-compose up --build
make cover              # Test with coverage + HTML report
```

### Code Generation

Proto → Go: `buf generate` (configured in `buf.gen.yaml`, output to `gen/proto/`)
SQL → Go: `sqlc generate` (configured in `sqlc.yaml`, output to `internal/repository/`)

Generated files are gitignored (proto) or committed (sqlc).

---

## 16. Testing Strategy

### Unit Tests (27 tests, no external dependencies)

| Package | What's tested |
|---|---|
| `matching` | Limit/market order matching, partial fills, STP, slippage |
| `events` | Bus publish/subscribe/unsubscribe |
| `telemetry` | Metrics registration, tracer init, health checks |
| `server` | (tested via integration tests with DB) |

### Integration Tests (tag: `integration`)

File: `internal/repository/repository_integration_test.go` — build tag `//go:build integration`

Uses `testcontainers-go` to spin up a real PostgreSQL 16 container per test run via `db.SetupTestDB(t)`. Tests CreateOrder, GetOrder, CancelOrder, and CreateTrade against actual SQL.

Run: `go test -tags=integration -race ./internal/repository/...`

### Fuzz Tests

File: `internal/matching/fuzz_test.go` — targeted fuzzing of the order book insertion and matching paths.

### Benchmarks

Files: `internal/matching/bench_test.go`, `internal/matching/benchmark_test.go`

Run: `go test -bench=. -benchtime=1x ./internal/matching/`

### Coverage

```bash
make cover        # HTML report
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
```

---

## 17. Production Fixes (Branch)

Branch `production-fixes` contains 6 merged commits plus uncommitted changes addressing production-readiness gaps:

### Completed Commits

| Commit | Task | Description |
|---|---|---|
| `e22ed65` | 1 | **Dockerfile Go version**: bumped from 1.23 → 1.26 to match `go.mod` and pick up latest security patches |
| `7b9a740` | 2 | **CI race flag**: added `-race` to `go test` in CI to detect data races on every run |
| `958231a` | 4 | **Request ID interceptor**: UUID injection into context + logging for request correlation across logs |
| `6fd634d` | 5 | **Event publish ordering**: moved `bus.Publish()` calls inside `persistOrderTx` to after `tx.Commit()` — prevents phantom trade events when the DB transaction rolls back |
| `dbc3b19` | 6 | **TradeFeed goroutine leak**: sender goroutines now select on `ctx.Done()` for clean shutdown; `sync.WaitGroup` ensures all goroutines exit before `TradeFeed` returns |
| `5f41bd0` | 3 | **Prometheus metrics**: added 6 metrics, health endpoints (`/healthz`, `/ready`), book depth goroutine, instrumented `CreateOrder` |

### Uncommitted Changes

| Task | Description |
|---|---|
| 8 | **OTel tracing**: `InitTracerProvider` with OTLP gRPC exporter to `otel-collector:4317`, `BatchSpanProcessor`, `otelgrpc.NewServerHandler()` wired via `grpc.StatsHandler`, `SubmitOrder` trace spans with order attributes, domain `String()` methods. Semconv module skipped (unresolvable in Go 1.26 due to module path conflict with parent `go.opentelemetry.io/otel` module). |
| 9 | **Integration CI job**: new `integration` job in `.github/workflows/ci.yml` running `go test -tags=integration -race`. Depends on lint+test; build job depends on all three. |

---

## 18. Known Limitations / What I'd Do at Scale

This section acknowledges what this project is **not** and what I'd change for production scale. Naming these explicitly is part of engineering judgment — knowing where your design stops working is as important as knowing where it works.

### 18.1 No Authentication / Authorization

The service currently has no auth middleware. Every client can access every RPC. No API keys, no JWT, no mTLS enforcement (though mTLS is configurable at the transport layer).

**At scale:** Add an authentication interceptor that validates a bearer token (e.g., JWT or opaque API key) on every request. The interceptor extracts the caller identity and enforces per-user/role authorization on each RPC. This is a server-side interceptor implementation — no changes to the matching engine or persistence layer.

### 18.2 Single-Process, Single-Book

The matching engine lives entirely in a single process. There's one `Engine` instance with one `map[string]*OrderBook` — no horizontal scaling, no sharding. All symbols share the same process memory.

**At scale:** Shard by symbol across multiple processes or hosts. A thin routing layer (gRPC gateway or proxy) maps `symbol` → shard. Each shard is an independent `Engine` instance. This also means no cross-symbol matching (which this engine doesn't support anyway).

### 18.3 No Rate Limiting

A malicious or buggy client can flood the service with orders, consuming CPU and memory with no backpressure.

**At scale:** Add token-bucket rate limiting per user or per IP at the interceptor level. This prevents runaway clients from degrading service for others. A simpler alternative: cap the number of open orders per user.

### 18.4 No Persistence for Book State

The order book is reconstructed from PostgreSQL on startup by replaying open orders. This works but means the book state is only as current as the last DB write. If the process crashes between matching an order and persisting the result, that trade is lost.

**At scale:** Use a WAL-based approach (e.g., Apache Kafka for the event log, or a replicated in-memory store like Redis with AOF) so matching and persistence are coupled through a durable log, not a database transaction on the hot path. Some exchanges use a "firm → done" two-phase model where a match is provisional until the WAL confirms.

### 18.5 No Realistic Order Throughput

The benchmark suite reports ~0.8µs per order in isolation. This does not account for gRPC deserialization, interceptor overhead, database writes, or event bus fan-out — just the matching engine hot path.

**At scale:** End-to-end profiling with realistic payload sizes and concurrent clients would be the first step. The gRPC + OTel + database pipeline is likely to be bottlenecked on PostgreSQL write throughput before the matching engine.

### 18.6 Single-Region, No HA

Docker Compose runs one copy of everything. There's no leader election, no replica, no active-active deployment.

**At scale:** Deploy behind a load balancer with health-check-based routing (already have `/healthz` and `/ready`). Use a distributed database (CockroachDB or Spanner) or active-passive PostgreSQL with streaming replication. The matching engine's in-memory state makes active-active challenging — symbol sharding (per §18.2) keeps each shard single-writer.

### 18.7 No Admin / Management API

There's no API for schema inspection, hot-reload config, draining the book, or pausing matching.

**At scale:** Add a small management gRPC service (`AdminService`) with RPCs for `DrainSymbol`, `ReloadConfig`, `GetEngineStats`, and `SetLogLevel`. These are not exposed to external clients and require separate auth.

---

## Appendix: Key Design Decisions

### Why an in-memory matching engine?

Latency. A matching engine that processes orders through a database round-trip would add 1-10ms per order. The in-memory approach achieves ~0.8µs per order. PostgreSQL is used as the system of record (durability, recovery, audit), not the matching hot path.

### Why no semconv module?

The `go.opentelemetry.io/otel/semconv` module path shares the prefix `go.opentelemetry.io/otel` with the main OTel module. In Go 1.26+, the module proxy resolves this to a subpackage of the main module rather than a separate module. The semconv module is effectively unreachable as a dependency. Workaround: define OTel resource attributes inline using raw `attribute.String("service.name", ...)`. This has no runtime impact — the attributes are identical to what semconv would produce.

### Why publish events after tx.Commit?

If events were published before the DB commit, a crash between Publish and Commit would produce phantom events — downstream consumers see a trade that never persisted. Publishing after commit guarantees that any consumed event corresponds to a committed transaction. This is an at-least-once delivery pattern: consumers must handle duplicates.

### Why self-trade prevention?

MiFID II (Markets in Financial Instruments Directive II) requires that market participants do not execute trades against themselves. The STP check in `fillOrdersAtLevel` prevents an institution's order from matching another order from the same `user_id`, ensuring compliance.
