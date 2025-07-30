.PHONY: help down init shell dev gen-swagger migrate-up migrate-down test test-short test-coverage test-race test-oracle test-handler test-btcrpc test-clean test-build test-timeout-fix lint type-check check

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "ICY Backend - Available commands:"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo

down: ## Stop and remove Docker containers with volumes
	docker compose down --remove-orphans --volumes

init: down ## Initialize database and start services
	@ make stop || true
	@ echo "Starting PostgreSQL container..."
	@ docker compose up -d
	@ echo "Waiting for PostgreSQL to be ready..."

shell: ## Enter devbox development shell
	@if ! command -v devbox >/dev/null 2>&1; then curl -fsSL https://get.jetpack.io/devbox | bash; fi
	@devbox install
	@devbox shell

dev: ## Start the development server
	@echo "Starting server..."
	@devbox run server

gen-swagger: ## Generate Swagger documentation
	swag init --parseDependency -g ./cmd/server/main.go
	
migrate-up: ## Run database migrations up
	devbox run migrate-up

migrate-down: ## Rollback all database migrations
	devbox run migrate-down

# Test targets
test: ## Run all tests with verbose output
	@echo "Running all tests..."
	@go test ./... -v

test-short: ## Run short tests (exclude integration tests)
	@echo "Running short tests (excluding integration tests)..."
	@go test ./... -v -short

test-coverage: ## Run tests with coverage report (generates coverage.html)
	@echo "Running tests with coverage..."
	@go test ./... -v -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-race: ## Run tests with race condition detection
	@echo "Running tests with race detection..."
	@go test ./... -v -race

test-oracle: ## Run Oracle package tests only
	@echo "Running Oracle tests..."
	@go test ./internal/oracle -v

test-handler: ## Run Handler package tests only
	@echo "Running Handler tests..."
	@go test ./internal/handler/... -v

test-btcrpc: ## Run BTC RPC package tests only
	@echo "Running BTC RPC tests..."
	@go test ./internal/btcrpc -v

test-clean: ## Clean test cache and coverage files
	@echo "Cleaning test cache and coverage files..."
	@go clean -testcache
	@rm -f coverage.out coverage.html

# Quality assurance targets
lint: ## Run Go linter (requires golangci-lint)
	@echo "Running linter..."
	@golangci-lint run ./...

type-check: ## Run Go type checking (build without output)
	@echo "Running type check..."
	@go build ./...

test-build: ## Test that all packages compile successfully
	@echo "Testing build compilation..."
	@go build ./... > /dev/null 2>&1 && echo "✅ All packages compile successfully" || echo "❌ Build compilation failed"

test-timeout-fix: ## Validate timeout fix implementation compiles
	@echo "Validating timeout fix implementation..."
	@go build ./internal/oracle ./internal/handler/swap > /dev/null 2>&1 && echo "✅ Timeout fix implementation compiles successfully" || echo "❌ Timeout fix compilation failed"

check: type-check lint test-short ## Run all quality checks (type-check + lint + short tests)
	@echo "All quality checks passed!"
