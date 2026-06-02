# PROJECT KNOWLEDGE BASE — order-engine

**Status:** Scaffolding in progress
**Repo:** `github.com/AlexPips/order-engine`
**Purpose:** Portfolio gRPC order management service to land a backend role in fintech/trading infrastructure.

> **This file is the source of truth.** Read it before doing anything in a new session. It is updated at the end of every meaningful work session.

---

## WHY THIS PROJECT EXISTS

`rssagg` was the Boot.dev RSS aggregator tutorial. It got privatized because:
- It was recognizable to senior Go engineers in <30s
- It was irrelevant to the target roles (fintech/trading)
- It had no gRPC, no Kafka, no streaming, no observability, no real concurrency patterns
- Pinning it was actively hurting the profile

`order-engine` replaces it. It directly targets the gap:
- **4 gRPC streaming patterns** (unary, server-stream, client-stream, bidi) — gRPC is in 4/5 target job descriptions
- **In-memory order matching engine** with price-time priority — directly mirrors trading system architecture (what GRS Trading, Exness, Plata Card, Exinity, RoboMarkets build)
- **Production observability** (OpenTelemetry + Prometheus) — every target job asks
- **sqlc + PostgreSQL** — type-safe data layer
- **testcontainers integration tests** — real Postgres, no mocks
- **Graceful shutdown** — what every backend interviewer wants to hear
- **CI, Docker, Migrations, Benchmarks, pprof** — full professional signal

Target roles this project is designed to help land:
- Plata Card (Processing Acquirer / Origination Delivery) — Cyprus, fintech
- FaceApp — Limassol, consumer Go backend
- Exness — Limassol, trading platform (stretch on years)
- GRS Recruitment (fintech client) — Limassol, trading
- Aeon Payment Technologies — Nicosia local, fintech
- Exinity (FXTM) — Limassol, HFT microservices
- RoboMarkets — Limassol, crypto broker
- gridX — energy/IoT, eBOS-aligned

Full plan: `/mnt/c/Users/AlexisP.EBOS.002/Projects/personal/grpc-order-service-plan.md`
User context: `/mnt/c/Users/AlexisP.EBOS.002/Projects/personal/AGENTS.md`

---

## TECH STACK (locked decisions)

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.23+ | User's primary CV language |
| RPC | `google.golang.org/grpc` | Standard |
| Proto tooling | `buf` | Lint + breaking-change detection + generation in one tool — what fintech shops use |
| Database | PostgreSQL 16 | Industry standard for transactional systems |
| Query layer | `sqlc` (NOT sqlx) | Type-safe, compile-time errors, idiomatic 2026 Go, matches CV tone |
| Migrations | `golang-migrate/migrate` | CLI + library, works in CI |
| Observability | OpenTelemetry + Prometheus | Every target job asks; OTel spans + Prom metrics is the default |
| Logging | `log/slog` (stdlib) | No vendor lock-in, JSON output |
| Config | env vars + `envconfig` | 12-factor |
| Testing | stdlib `testing` + `testcontainers-go` | Real Postgres in integration tests, no mocks |
| Lint | `golangci-lint` | Standard |
| CI | GitHub Actions | Repo host |
| Container | Docker + docker-compose | One-command spin-up |
| Numbers | `shopspring/decimal` | Never use float64 for prices/quantities |

---

## ARCHITECTURE

```
gRPC client (insecure/TLS/mTLS)
    │
    ▼
┌────────────────────────────────────────────────────────┐
│  gRPC Server (cmd/server/main.go)                     │
│  ├─ Interceptors: OTel tracing, recovery, logging     │
│  ├─ Handlers (internal/server/)                       │
│  │   ├─ CreateOrder (unary)                           │
│  │   ├─ CancelOrder (unary)                           │
│  │   ├─ GetOrder (unary)                              │
│  │   ├─ GetOrderBook (unary)                          │
│  │   ├─ StreamOrderUpdates (server-stream)            │
│  │   ├─ BatchCreateOrders (client-stream)             │
│  │   └─ TradeFeed (bidi-stream)                       │
│  ├─ Matching Engine (internal/matching/)              │
│  │   ├─ In-memory order book (per symbol)             │
│  │   ├─ Price-time priority queue                     │
│  │   ├─ Limit + market orders                         │
│  │   ├─ Partial fills                                 │
│  │   └─ Trade event emission                          │
│  ├─ Event Bus (internal/events/)                      │
│  │   └─ Pub/sub for streaming RPCs + OTel spans       │
│  ├─ Repository (internal/repository/, sqlc-generated) │
│  │   ├─ Persist orders (OPEN → FILLED/PARTIAL/CANCEL) │
│  │   ├─ Persist trades (executed matches)             │
│  │   └─ Audit log                                     │
│  └─ Telemetry (internal/telemetry/)                   │
│      ├─ OTel tracer setup                             │
│      ├─ Prometheus metrics registry                   │
│      └─ slog JSON handler                             │
└────────────────────────────────────────────────────────┘
    │
    ▼
PostgreSQL (docker / k8s)  ◄── migrations via golang-migrate
```

---

## DOMAIN MODEL

```go
// Order
type Order struct {
    ID        OrderID
    UserID    UserID
    Symbol    string      // e.g. "BTCUSD"
    Side      Side        // BUY | SELL
    Type      OrderType   // LIMIT | MARKET
    Price     decimal.Decimal  // zero for market orders
    Quantity  decimal.Decimal
    Status    OrderStatus // NEW | PARTIAL | FILLED | CANCELED | REJECTED
    CreatedAt time.Time
    UpdatedAt time.Time
    FilledQty decimal.Decimal
}

// Trade (a match between two orders)
type Trade struct {
    ID         TradeID
    Symbol     string
    BuyOrderID OrderID
    SellOrderID OrderID
    Price      decimal.Decimal
    Quantity   decimal.Decimal
    ExecutedAt time.Time
}

// OrderBook snapshot
type OrderBookSnapshot struct {
    Symbol   string
    Bids     []PriceLevel  // sorted desc by price
    Asks     []PriceLevel  // sorted asc by price
    Sequence uint64        // for stream resume
}

type PriceLevel struct {
    Price    decimal.Decimal
    Quantity decimal.Decimal
    OrderCount int
}
```

---

## PROJECT STRUCTURE (target)

```
order-engine/
├── proto/
│   └── order/v1/
│       └── order.proto              # Protobuf definitions (all 4 RPC patterns)
├── gen/                              # Generated code (gitignored)
├── cmd/
│   └── server/
│       └── main.go                   # Entry point: config → DB → telemetry → matching → server → graceful shutdown
├── internal/
│   ├── server/                       # gRPC handler implementations
│   │   ├── order_service.go
│   │   └── interceptors.go
│   ├── matching/                     # In-memory order matching engine
│   │   ├── engine.go
│   │   ├── book.go                   # Per-symbol order book
│   │   ├── queue.go                  # Price-time priority queue
│   │   ├── engine_test.go
│   │   └── benchmark_test.go
│   ├── domain/                       # Core types (Order, Trade, OrderBook)
│   │   ├── order.go
│   │   ├── trade.go
│   │   └── id.go
│   ├── repository/                   # sqlc-generated + interface
│   │   ├── orders.sql.go             # generated
│   │   ├── trades.sql.go              # generated
│   │   └── repository.go             # interface
│   ├── events/                       # In-process pub/sub for streaming RPCs
│   │   └── bus.go
│   ├── telemetry/                    # OTel + Prometheus setup
│   │   ├── otel.go
│   │   ├── prom.go
│   │   └── logger.go
│   ├── config/                       # env loading + validation
│   │   └── config.go
│   └── db/                           # DB connection pool + health
│       └── pool.go
├── migrations/                       # golang-migrate files
│   ├── 0001_create_orders.up.sql
│   ├── 0001_create_orders.down.sql
│   ├── 0002_create_trades.up.sql
│   └── 0002_create_trades.down.sql
├── queries/                          # SQL files for sqlc
│   ├── orders.sql
│   └── trades.sql
├── deployments/
│   ├── Dockerfile                    # Multi-stage build
│   └── docker-compose.yml            # Postgres + service
├── .github/
│   └── workflows/
│       └── ci.yml                    # lint → test → build on push
├── buf.gen.yaml                      # buf codegen pipeline
├── buf.yaml                          # buf module config
├── sqlc.yaml                         # sqlc codegen config
├── .golangci.yml                     # linter rules
├── Makefile
├── go.mod
├── go.sum
├── LICENSE                           # MIT
├── README.md
├── AGENTS.md                         # THIS FILE
└── .gitignore
```

---

## CURRENT STATE — what is done vs pending

### Done
- [x] Repo created on GitHub: `github.com/AlexPips/order-engine`
- [x] Local directory created
- [x] `README.md` — full project description, architecture diagram, RPC examples, stack table
- [x] `LICENSE` — MIT
- [x] `.gitignore` — Go + generated + IDE patterns
- [x] `Makefile` — all targets from README wired up
- [x] `AGENTS.md` — this file (project memory)

### Next up (in order)
1. `go mod init github.com/AlexPips/order-engine` + initial dependencies
2. `buf` setup: `buf.yaml`, `buf.gen.yaml`
3. `proto/order/v1/order.proto` — define messages + all 4 RPC patterns
4. Generate proto → first commit with generated code review
5. `sqlc.yaml` + initial schema
6. `migrations/0001_create_orders.sql` + `0002_create_trades.sql`
7. `internal/domain/` — Order, Trade, OrderBook, IDs (no external deps yet)
8. `internal/matching/` — the engine (this is the centerpiece, gets the most attention)
   - Start with the data structures: order book per symbol
   - Add Limit order insertion + price-time priority queue
   - Add matching logic (price-time priority match)
   - Add Market order handling
   - Add partial fills
   - Add trade event emission
   - **Unit tests for every method** + table-driven tests
   - **Benchmark: 1 symbol 10 concurrent, 1 symbol 1000 orders, 100 symbols**
9. `internal/events/` — pub/sub bus
10. `internal/telemetry/` — OTel + Prometheus
11. `internal/repository/` — sqlc + interface
12. `internal/db/` — pool + health
13. `internal/config/` — envconfig
14. `internal/server/` — gRPC handlers wrapping engine + repo
15. `cmd/server/main.go` — wire it all up + graceful shutdown
16. Integration tests with testcontainers
17. `deployments/Dockerfile` + `docker-compose.yml`
18. `.golangci.yml` + linter config
19. `.github/workflows/ci.yml` — lint + test + build on push
20. Final polish: example client, grpcurl recipes in README, architecture diagram commit

---

## CONVENTIONS

### Go style
- Package names: lowercase, single word, no underscores (`matching`, `repository`, not `orderMatching`)
- Errors: always wrapped with `%w`, never silently dropped
- Context: first parameter of any function that does I/O
- Logging: `slog.InfoContext(ctx, "msg", "key", val, ...)` — never `log.Println` in production paths
- IDs: typed string aliases (`type OrderID string`) — prevents ID mix-ups at compile time
- Numbers: `decimal.Decimal` for all prices/quantities, NEVER `float64`
- Mutex discipline: hold locks for the shortest possible scope, document any held across I/O

### File naming
- One type per file when types are large (`order.go`, `trade.go`)
- Test files: `<thing>_test.go` in the same package
- Integration tests: `<thing>_integration_test.go` with `//go:build integration` tag

### Commit messages
- Imperative mood: "Add price-time priority matching" not "Added"
- One logical change per commit
- Format: `<scope>: <change>` — e.g. `matching: add partial fill support`, `proto: add Trade message`

### Branches
- `main` is always green (CI passing)
- Feature work on `feat/<name>` branches
- Commit early, commit often, rebase before merging

### Pull requests (when going public)
- The README, Makefile, and AGENTS.md get updated in the same PR as the code change
- Every PR has a working `make test && make lint`

---

## BUILD COMMANDS (all of these should work after the project is scaffolded)

```bash
make help                # List all targets
make proto               # Lint + generate Go from proto
make sqlc                # Generate type-safe Go from SQL
make migrate-up          # Apply DB migrations
make migrate-down        # Roll back last migration
make build               # Compile server binary
make run                 # Run server (requires DB)
make test                # All tests
make test-unit           # Unit tests only (no Docker)
make test-integration    # Integration tests (Docker required)
make bench               # Matching engine benchmarks
make cover               # Coverage report (HTML)
make lint                # golangci-lint
make vet                 # go vet
make tidy                # go mod tidy
make docker-build        # Build image
make docker-up           # Spin up Postgres + service
make docker-down         # Stop stack
make clean               # Remove build artifacts
```

---

## KEY DESIGN DECISIONS (don't re-litigate these)

1. **sqlc over sqlx/gorm** — type safety, no runtime overhead, idiomatic. The plan said sqlx; we're using sqlc.
2. **In-memory matching engine, not database-driven** — speed + simplicity. Persistence is the *result* of matches (trades table), not the source of truth for the book. The book is rebuilt from the DB on startup.
3. **Decimal for money, not float64** — non-negotiable in fintech.
4. **buf over raw protoc** — schema governance matters in teams; we're a team of one but the signal is real.
5. **testcontainers over mocks for integration tests** — mocks lie; real Postgres doesn't.
6. **OpenTelemetry SDK, not just metrics or just traces** — full observability story.
7. **No REST gateway** — pure gRPC. A REST gateway can be added later without renaming the service.
8. **MIT license, not Apache 2.0** — simpler, fine for a portfolio piece.

---

## MENTOR STYLE (carry over from user's personal AGENTS.md)

The user has explicitly asked for **ruthless, no-sugar-coating feedback**. If a design choice is weak, say so. If a pattern is wrong, say so. Do not soften criticism with hedge words. Reasoning still required — "this is bad" is not enough; "this is bad *because* X, *specifically*, *do Y instead*" is the standard.

---

## DON'Ts (things that have already been decided NOT to do)

- Don't add a REST gateway (yet) — pure gRPC
- Don't add authentication in v1 — note it as a follow-up; mTLS is documented in README
- Don't use float64 anywhere
- Don't add an ORM
- Don't use interface{} or any — types are concrete
- Don't commit generated code (it goes in `gen/` which is gitignored)
- Don't use a tutorial-style `handler_user.go` / `handler_feed.go` file naming (rssagg trauma)
- Don't add features that aren't in the roadmap without consulting the user
- Don't ship without tests
- Don't push to GitHub without user approval

---

## NOTES

- Local Go version: whatever is installed (check `go version`). Target 1.23+.
- The `personal/grpc-order-service-plan.md` is the ORIGINAL plan. This file supersedes it — defer to AGENTS.md if they conflict.
- If a new session loads this file, run `git log --oneline -20` to see what's been done since the last update.
