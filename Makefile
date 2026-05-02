.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ── Dev ───────────────────────────────────────────────────────────────────────
.PHONY: dev
dev: ## Run the server with hot-reload (requires air)
	air -c .air.toml

.PHONY: run
run: ## Run the server without hot-reload
	go run ./cmd/server/main.go

.PHONY: build
build: ## Build the binary into bin/dev-forge
	go build -o bin/dev-forge ./cmd/server/main.go

# ── Test ──────────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run all tests
	go test -v -race ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# ── Lint ──────────────────────────────────────────────────────────────────────
.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go files
	gofmt -w .
	goimports -w .

# ── Migrations ────────────────────────────────────────────────────────────────
MIGRATIONS_DIR := ./migrations
DB_URL ?= postgres://postgres:postgres@localhost:5432/dev_forge?sslmode=disable

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: ## Rollback the last migration
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create new migration (usage: make migrate-create name=add_users)
	@[ "$(name)" ] || ( echo "Usage: make migrate-create name=<migration_name>"; exit 1 )
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

.PHONY: migrate-version
migrate-version: ## Show current migration version
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" version

.PHONY: migrate-force
migrate-force: ## Force migration version (usage: make migrate-force version=1)
	@[ "$(version)" ] || ( echo "Usage: make migrate-force version=<N>"; exit 1 )
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" force $(version)

# ── Docker / Infra ────────────────────────────────────────────────────────────
COMPOSE_FILE := deployments/docker-compose.yml

.PHONY: docker-up
docker-up: ## Start all infra (PostgreSQL, Redis, Zitadel)
	docker compose -f $(COMPOSE_FILE) up -d

.PHONY: docker-down
docker-down: ## Stop and remove all infra containers
	docker compose -f $(COMPOSE_FILE) down

.PHONY: docker-logs
docker-logs: ## Tail logs from all infra containers
	docker compose -f $(COMPOSE_FILE) logs -f

.PHONY: docker-clean
docker-clean: ## Stop containers and remove volumes (WARNING: deletes data)
	docker compose -f $(COMPOSE_FILE) down -v

# ── Full setup ────────────────────────────────────────────────────────────────
.PHONY: setup
setup: docker-up migrate-up ## Start infra and apply migrations
	@echo "Setup complete. Run 'make run' to start the server."

.PHONY: reset
reset: docker-clean docker-up migrate-up ## Reset everything (WARNING: deletes all data)
