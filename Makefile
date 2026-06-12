.PHONY: help build run test test-unit bench vet tidy clean

help: ## List all targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

## --- Build ---

build: ## Compile server binary
	go build -o bin/server ./cmd/server/

run: build ## Build and run server
	./bin/server

clean: ## Remove build artifacts
	rm -rf bin/

## --- Test ---

test: ## Run all tests
	go test ./...

test-unit: ## Run unit tests only (exclude integration tests)
	go test $$(shell go list ./... | grep -v /integration)

test-integration: ## Run integration tests (requires Docker + migrate CLI)
	go test -tags=integration -count=1 ./...

bench: ## Run matching engine benchmarks
	go test -bench=. -benchtime=1x ./internal/matching/

## --- Code Quality ---

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy module dependencies
	go mod tidy

## --- Proto (requires buf + protoc plugins) ---

proto: ## Lint and generate Go code from proto definitions
	@if ! command -v buf >/dev/null 2>&1; then \
		echo "ERROR: buf not installed. Run: go install github.com/bufbuild/buf/cmd/buf@latest"; \
		exit 1; \
	fi
	buf lint proto/
	buf generate

## --- Database (requires golang-migrate) ---

MIGRATE_CMD = migrate

migrate-up: ## Apply all pending migrations
	@if ! command -v $(MIGRATE_CMD) >/dev/null 2>&1; then \
		echo "ERROR: migrate not installed. Run: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
		exit 1; \
	fi
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "ERROR: DATABASE_URL not set"; \
		exit 1; \
	fi
	$(MIGRATE_CMD) -path migrations -database "$(DATABASE_URL)" up

migrate-down: ## Roll back last migration
	@if ! command -v $(MIGRATE_CMD) >/dev/null 2>&1; then \
		echo "ERROR: migrate not installed. Run: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
		exit 1; \
	fi
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "ERROR: DATABASE_URL not set"; \
		exit 1; \
	fi
	$(MIGRATE_CMD) -path migrations -database "$(DATABASE_URL)" down 1

## --- SQLC ---

sqlc: ## Regenerate type-safe Go code from SQL queries
	@if ! command -v sqlc >/dev/null 2>&1; then \
		echo "ERROR: sqlc not installed. Run: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi
	sqlc generate

## --- Lint (requires golangci-lint) ---

lint: ## Run golangci-lint
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "ERROR: golangci-lint not installed. See https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	fi
	golangci-lint run

## --- Docker ---

docker-build: ## Build Docker image
	docker build -t order-engine -f deployments/Dockerfile .

docker-up: ## Start Postgres + service
	docker compose -f deployments/docker-compose.yml up --build

docker-down: ## Stop stack
	docker compose -f deployments/docker-compose.yml down

## --- Coverage ---

cover: ## Run tests with coverage and open HTML report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
