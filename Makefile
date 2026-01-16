# CJADC2 Platform Makefile
# Build, test, and run commands for the CJADC2 platform

.PHONY: help build up down logs test lint clean dev infra agents api ui \
        build-agents build-api build-ui \
        logs-nats logs-postgres logs-opa logs-agents logs-api \
        db-shell nats-shell opa-shell \
        db-migrate db-reset \
        demo status

# Default target
.DEFAULT_GOAL := help

# Colors for output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

#------------------------------------------------------------------------------
# Help
#------------------------------------------------------------------------------

help: ## Show this help message
	@echo "$(CYAN)CJADC2 Platform$(RESET) - Build and Run Commands"
	@echo ""
	@echo "$(GREEN)Usage:$(RESET)"
	@echo "  make [target]"
	@echo ""
	@echo "$(GREEN)Targets:$(RESET)"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_-]+:.*?##/ { printf "  $(CYAN)%-15s$(RESET) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Quick Start:$(RESET)"
	@echo "  make up       - Start all services"
	@echo "  make logs     - View all logs"
	@echo "  make down     - Stop all services"

#------------------------------------------------------------------------------
# Build
#------------------------------------------------------------------------------

build: ## Build all containers
	@echo "$(CYAN)Building all containers...$(RESET)"
	docker compose build

build-agents: ## Build agent containers only
	@echo "$(CYAN)Building agent containers...$(RESET)"
	docker compose build sensor-sim classifier correlator planner authorizer effector

build-api: ## Build API gateway container
	@echo "$(CYAN)Building API gateway container...$(RESET)"
	docker compose build api-gateway

build-ui: ## Build UI container
	@echo "$(CYAN)Building UI container...$(RESET)"
	docker compose build ui

build-nocache: ## Build all containers without cache
	@echo "$(CYAN)Building all containers (no cache)...$(RESET)"
	docker compose build --no-cache

#------------------------------------------------------------------------------
# Run
#------------------------------------------------------------------------------

up: ## Start all services with docker compose
	@echo "$(CYAN)Starting CJADC2 platform...$(RESET)"
	docker compose up -d
	@echo ""
	@echo "$(GREEN)Services starting...$(RESET)"
	@echo ""
	@echo "  Web UI:        http://localhost:3000"
	@echo "  API Gateway:   http://localhost:8080"
	@echo "  NATS Monitor:  http://localhost:8222"
	@echo "  Prometheus:    http://localhost:9090"
	@echo "  Jaeger:        http://localhost:16686"
	@echo "  OPA:           http://localhost:8181"
	@echo ""
	@echo "Run 'make logs' to view logs, 'make status' to check health"

up-build: ## Build and start all services
	@echo "$(CYAN)Building and starting CJADC2 platform...$(RESET)"
	docker compose up -d --build

down: ## Stop all services
	@echo "$(CYAN)Stopping CJADC2 platform...$(RESET)"
	docker compose down

restart: ## Restart all services
	@echo "$(CYAN)Restarting CJADC2 platform...$(RESET)"
	docker compose restart

#------------------------------------------------------------------------------
# Development
#------------------------------------------------------------------------------

infra: ## Start infrastructure only (NATS, PostgreSQL, OPA, observability)
	@echo "$(CYAN)Starting infrastructure services...$(RESET)"
	docker compose up -d nats postgres opa prometheus jaeger
	@echo ""
	@echo "$(GREEN)Infrastructure ready$(RESET)"
	@echo "  NATS:       nats://localhost:4222"
	@echo "  PostgreSQL: postgres://localhost:5432/cjadc2"
	@echo "  OPA:        http://localhost:8181"

dev: infra ## Start infrastructure for local development
	@echo ""
	@echo "$(YELLOW)Run agents locally:$(RESET)"
	@echo "  go run ./cmd/agents/sensor"
	@echo "  go run ./cmd/agents/classifier"
	@echo "  go run ./cmd/agents/correlator"
	@echo "  go run ./cmd/agents/planner"
	@echo "  go run ./cmd/agents/authorizer"
	@echo "  go run ./cmd/agents/effector"
	@echo ""
	@echo "$(YELLOW)Run API gateway locally:$(RESET)"
	@echo "  go run ./cmd/api-gateway"
	@echo ""
	@echo "$(YELLOW)Run UI in development mode:$(RESET)"
	@echo "  cd ui && npm install && npm run dev"

#------------------------------------------------------------------------------
# Logs
#------------------------------------------------------------------------------

logs: ## View all logs (follow mode)
	docker compose logs -f

logs-nats: ## View NATS logs
	docker compose logs -f nats

logs-postgres: ## View PostgreSQL logs
	docker compose logs -f postgres

logs-opa: ## View OPA logs
	docker compose logs -f opa

logs-agents: ## View all agent logs
	docker compose logs -f sensor-sim classifier correlator planner authorizer effector

logs-api: ## View API gateway logs
	docker compose logs -f api-gateway

logs-ui: ## View UI logs
	docker compose logs -f ui

logs-tail: ## View last 100 lines of all logs
	docker compose logs --tail=100

#------------------------------------------------------------------------------
# Testing
#------------------------------------------------------------------------------

test: ## Run all tests
	@echo "$(CYAN)Running tests...$(RESET)"
	go test -v ./...

test-cover: ## Run tests with coverage
	@echo "$(CYAN)Running tests with coverage...$(RESET)"
	go test -v -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report: coverage.html$(RESET)"

test-race: ## Run tests with race detector
	@echo "$(CYAN)Running tests with race detector...$(RESET)"
	go test -v -race ./...

test-integration: up ## Run integration tests (requires running services)
	@echo "$(CYAN)Running integration tests...$(RESET)"
	go test -v -tags=integration ./tests/...

#------------------------------------------------------------------------------
# Linting
#------------------------------------------------------------------------------

lint: ## Run linters
	@echo "$(CYAN)Running linters...$(RESET)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not installed. Running go vet instead...$(RESET)"; \
		go vet ./...; \
	fi

lint-fix: ## Run linters with auto-fix
	@echo "$(CYAN)Running linters with auto-fix...$(RESET)"
	golangci-lint run --fix ./...

fmt: ## Format Go code
	@echo "$(CYAN)Formatting Go code...$(RESET)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(CYAN)Running go vet...$(RESET)"
	go vet ./...

#------------------------------------------------------------------------------
# Database
#------------------------------------------------------------------------------

db-shell: ## Open PostgreSQL shell
	@echo "$(CYAN)Opening PostgreSQL shell...$(RESET)"
	docker compose exec postgres psql -U cjadc2 -d cjadc2

db-migrate: ## Run database migrations
	@echo "$(CYAN)Running migrations...$(RESET)"
	docker compose exec postgres psql -U cjadc2 -d cjadc2 -f /docker-entrypoint-initdb.d/001_initial_schema.sql

db-reset: ## Reset database (WARNING: destroys data)
	@echo "$(RED)WARNING: This will destroy all data!$(RESET)"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ]
	docker compose down -v postgres
	docker compose up -d postgres
	@echo "$(GREEN)Database reset complete$(RESET)"

#------------------------------------------------------------------------------
# Service Shells
#------------------------------------------------------------------------------

nats-shell: ## Open NATS CLI (requires nats-cli installed)
	@echo "$(CYAN)Opening NATS CLI...$(RESET)"
	@if command -v nats >/dev/null 2>&1; then \
		nats -s localhost:4222 --user admin --password admin-secret; \
	else \
		echo "$(YELLOW)nats-cli not installed. Install from: https://github.com/nats-io/natscli$(RESET)"; \
	fi

opa-shell: ## Open OPA REPL
	@echo "$(CYAN)Opening OPA REPL...$(RESET)"
	docker compose exec opa /opa run --server=false /bundles/cjadc2

#------------------------------------------------------------------------------
# Cleanup
#------------------------------------------------------------------------------

clean: ## Clean up containers, volumes, and build artifacts
	@echo "$(CYAN)Cleaning up...$(RESET)"
	docker compose down -v --remove-orphans
	rm -f coverage.out coverage.html
	go clean -cache -testcache
	@echo "$(GREEN)Cleanup complete$(RESET)"

clean-images: ## Remove all CJADC2 images
	@echo "$(CYAN)Removing CJADC2 images...$(RESET)"
	docker images | grep cjadc2 | awk '{print $$3}' | xargs -r docker rmi -f

prune: ## Prune unused Docker resources
	@echo "$(CYAN)Pruning Docker resources...$(RESET)"
	docker system prune -f

#------------------------------------------------------------------------------
# Status and Monitoring
#------------------------------------------------------------------------------

status: ## Show status of all services
	@echo "$(CYAN)Service Status$(RESET)"
	@echo ""
	@docker compose ps
	@echo ""
	@echo "$(CYAN)Health Checks$(RESET)"
	@echo ""
	@echo -n "  NATS:       "; curl -sf http://localhost:8222/healthz >/dev/null && echo "$(GREEN)healthy$(RESET)" || echo "$(RED)unhealthy$(RESET)"
	@echo -n "  PostgreSQL: "; docker compose exec -T postgres pg_isready -U cjadc2 >/dev/null 2>&1 && echo "$(GREEN)healthy$(RESET)" || echo "$(RED)unhealthy$(RESET)"
	@echo -n "  OPA:        "; curl -sf http://localhost:8181/health >/dev/null && echo "$(GREEN)healthy$(RESET)" || echo "$(RED)unhealthy$(RESET)"
	@echo -n "  API:        "; curl -sf http://localhost:8080/health >/dev/null && echo "$(GREEN)healthy$(RESET)" || echo "$(RED)unhealthy$(RESET)"
	@echo -n "  UI:         "; curl -sf http://localhost:3000 >/dev/null && echo "$(GREEN)healthy$(RESET)" || echo "$(RED)unhealthy$(RESET)"

streams: ## Show NATS JetStream streams
	@echo "$(CYAN)NATS JetStream Streams$(RESET)"
	@curl -sf http://localhost:8222/jsz?streams=true 2>/dev/null | jq '.streams[] | {name: .name, messages: .state.messages, bytes: .state.bytes}' 2>/dev/null || echo "$(YELLOW)NATS not available$(RESET)"

metrics: ## Show key metrics
	@echo "$(CYAN)Key Metrics$(RESET)"
	@curl -sf http://localhost:8080/metrics 2>/dev/null | grep -E "^(api_requests|agent_messages)" | head -20 || echo "$(YELLOW)Metrics not available$(RESET)"

#------------------------------------------------------------------------------
# Demo
#------------------------------------------------------------------------------

demo: ## Run the demo script
	@echo "$(CYAN)Running demo...$(RESET)"
	./scripts/demo.sh

demo-tracks: ## Show current tracks
	@echo "$(CYAN)Current Tracks$(RESET)"
	@curl -sf http://localhost:8080/api/v1/tracks | jq '.tracks[] | {id: .external_track_id, classification: .classification, threat: .threat_level, position: .position}' 2>/dev/null || echo "$(YELLOW)API not available$(RESET)"

demo-proposals: ## Show pending proposals
	@echo "$(CYAN)Pending Proposals$(RESET)"
	@curl -sf http://localhost:8080/api/v1/proposals?status=pending | jq '.proposals[] | {id: .proposal_id, track: .external_track_id, action: .action_type, priority: .priority}' 2>/dev/null || echo "$(YELLOW)API not available$(RESET)"

#------------------------------------------------------------------------------
# Dependencies
#------------------------------------------------------------------------------

deps: ## Download Go dependencies
	@echo "$(CYAN)Downloading dependencies...$(RESET)"
	go mod download

deps-tidy: ## Tidy Go module
	@echo "$(CYAN)Tidying Go module...$(RESET)"
	go mod tidy

deps-update: ## Update Go dependencies
	@echo "$(CYAN)Updating dependencies...$(RESET)"
	go get -u ./...
	go mod tidy

#------------------------------------------------------------------------------
# Documentation
#------------------------------------------------------------------------------

docs: ## Open documentation in browser
	@echo "$(CYAN)Opening documentation...$(RESET)"
	@if command -v open >/dev/null 2>&1; then \
		open README.md; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open README.md; \
	else \
		echo "Open README.md manually"; \
	fi

docs-serve: ## Serve documentation locally (requires mdbook or similar)
	@echo "$(YELLOW)Documentation serving not configured$(RESET)"
	@echo "View docs at: docs/ARCHITECTURE.md, docs/API.md"
