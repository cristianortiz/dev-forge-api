.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ── Dev ───────────────────────────────────────────────────────────────────────
.PHONY: dev
dev: ## Run the server with hot-reload (requires air)
	@set -a && . ./.env && set +a && air -c .air.toml

.PHONY: run
run: ## Run the server without hot-reload
	@set -a && . ./.env && set +a && go run ./cmd/server/

.PHONY: build
build: ## Build the binary into bin/dev-forge
	go build -o bin/dev-forge ./cmd/server/

# ── Test ──────────────────────────────────────────────────────────────────────
# Only packages that actually contain test files (avoids skewing total with untested adapters/cmd)
TEST_PKGS := $(shell go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... 2>/dev/null)

.PHONY: test
test: ## Run all tests with per-package and total coverage
	go test -v -race -coverprofile=coverage.out -covermode=atomic $(TEST_PKGS)
	@echo ""
	@go tool cover -func=coverage.out | grep "^total:"

.PHONY: test-coverage
test-coverage: ## Run tests with total coverage summary + HTML report
	go test -v -race -coverprofile=coverage.out -covermode=atomic $(TEST_PKGS)
	@go tool cover -func=coverage.out | grep "^total:"
	go tool cover -html=coverage.out

.PHONY: test-check
test-check: ## CI: fail if total coverage < 70% (tested packages only)
	@go test -race -coverprofile=coverage.out -covermode=atomic $(TEST_PKGS) > /dev/null
	@TOTAL=$$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	echo "Coverage: $${TOTAL}%"; \
	awk -v t="$$TOTAL" 'BEGIN { if (t+0 < 70) { print "FAIL: coverage " t "% is below the 70% threshold"; exit 1 } else { print "OK: coverage above threshold" } }'

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

.PHONY: seed
seed: ## Insert built-in project templates (idempotent)
	@set -a && . ./.env && set +a && go run ./cmd/seed/

.PHONY: swagger
swagger: ## Re-generate OpenAPI spec from handler annotations (requires swag CLI)
	swag init \
	  --generalInfo docs.go \
	  --dir cmd/server,internal/template/adapters/handler,internal/auth/adapters/handler \
	  --output docs/swagger

.PHONY: reset
reset: docker-clean docker-up migrate-up ## Reset everything (WARNING: deletes all data)
