# KilnLab Makefile — run `make help` for targets.

.DEFAULT_GOAL := help
.PHONY: help setup install-tools dev dev-api dev-web db db-stop db-reset \
        build build-web build-api sqlc docker-up docker-down docker-logs \
        test test-unit lint fmt tidy clean

CYAN  := \033[0;36m
RESET := \033[0m

GO      := go
PNPM    := pnpm
DC      := docker compose
WEB_DIR := web
SQLC    := $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

-include .env
export

TEST_DATABASE_URL ?= postgres://kilnlab:kilnlab@localhost:5433/kilnlab_test?sslmode=disable

##@ Setup

setup: ## First-time setup: create .env, install frontend deps
	@[ -f .env ] || { cp .env.example .env; printf '%b\n' "$(CYAN)Created .env from .env.example$(RESET)"; }
	@cd $(WEB_DIR) && $(PNPM) install

install-tools: ## Install Go/Node/pnpm from .tool-versions via asdf
	@asdf install

##@ Development

dev: db ## Start Postgres, the Go API (:8080) and Vite (:5173)
	@$(MAKE) -j2 dev-api dev-web

dev-api: ## Run the Go server (air for hot reload if installed)
	@if command -v air > /dev/null 2>&1; then air; else $(GO) run ./cmd/server; fi

dev-web: ## Run the Vite dev server (proxies /api to :8080)
	@cd $(WEB_DIR) && $(PNPM) run dev --host

db: ## Start Postgres in Docker
	@$(DC) up db -d --wait
	@printf '%b\n' "$(CYAN)Postgres ready on :5433$(RESET)"

db-stop: ## Stop Postgres
	@$(DC) stop db

db-reset: ## Destroy and recreate the dev database volume (deletes all data)
	@$(DC) down -v
	@$(MAKE) db

sqlc: ## Regenerate internal/db from migrations + queries
	@$(SQLC) generate

##@ Build

build: build-web build-api ## Build the SPA then the Go binary

build-web: ## Build the React SPA into internal/webembed/dist
	@cd $(WEB_DIR) && $(PNPM) install --frozen-lockfile && $(PNPM) run build

build-api: ## Compile ./kilnlab
	@$(GO) build -o kilnlab ./cmd/server

docker-up: ## Build and start the full stack in Docker
	@$(DC) up --build -d

docker-down: ## Stop the Docker stack (keeps volumes)
	@$(DC) down

docker-logs: ## Tail Docker logs
	@$(DC) logs -f

##@ Quality

test: db ## Run all Go tests, including DB integration tests
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) $(GO) test -p 1 -count=1 ./...

test-unit: ## Run Go tests that don't need Postgres
	@env -u TEST_DATABASE_URL $(GO) test ./...

lint: ## go vet + gofmt check + frontend type-check
	@$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	@cd $(WEB_DIR) && $(PNPM) exec tsc

fmt: ## Format Go and frontend code
	@$(GO) fmt ./...
	@cd $(WEB_DIR) && $(PNPM) exec prettier --write "src/**/*.{ts,tsx}" 2>/dev/null || true

tidy: ## go mod tidy
	@$(GO) mod tidy

clean: ## Remove build artifacts
	@rm -f kilnlab
	@find internal/webembed/dist -mindepth 1 ! -name placeholder.txt -delete

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make $(CYAN)<target>$(RESET)\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  $(CYAN)%-14s$(RESET) %s\n", $$1, $$2 } \
		/^##@/ { printf "\n$(CYAN)%s$(RESET)\n", substr($$0, 5) }' $(MAKEFILE_LIST)
