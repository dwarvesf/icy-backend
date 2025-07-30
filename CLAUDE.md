# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Environment Setup
- `make shell` - Create isolated development environment using devbox
- `make init` - Initialize database and start Docker services

### Development
- `make dev` - Start the development server (runs on port 3000)
- `devbox run server` - Alternative way to run the server directly

### Database Management
- `make migrate-up` - Run database migrations
- `make migrate-down` - Rollback all migrations
- `devbox run migrate-up` - Run migrations via devbox
- `devbox run migrate-down` - Rollback migrations via devbox

### API Documentation
- `make gen-swagger` - Generate Swagger documentation from code annotations
- Access Swagger UI at `/swagger/index.html` when server is running

### Docker Management
- `make down` - Stop and remove Docker containers with volumes

## Testing

The project uses Ginkgo/Gomega for BDD-style testing:
- Test files follow the `*_test.go` pattern
- Use `make test` to run all tests with verbose output
- Use `make test-short` for quick testing (excludes integration tests)
- Use `make test-coverage` to generate coverage reports
- Test suites are defined with Ginkgo's `Describe` and `It` blocks

### Available Test Commands
- `make test` - Run all tests with verbose output
- `make test-short` - Run short tests (exclude integration tests)
- `make test-coverage` - Run tests with coverage report (generates coverage.html)
- `make test-race` - Run tests with race condition detection
- `make test-oracle` - Run Oracle package tests only
- `make test-handler` - Run Handler package tests only
- `make test-btcrpc` - Run BTC RPC package tests only
- `make test-build` - Test that all packages compile successfully
- `make test-timeout-fix` - Validate timeout fix implementation compiles
- `make test-clean` - Clean test cache and coverage files

## Architecture Overview

### Core Components
This is a Go-based cryptocurrency swap backend service with the following key components:

**Main Application Flow:**
- `cmd/server/main.go` - Application entry point
- `internal/server/server.go` - Server initialization and cron job setup

**Data Layer:**
- `internal/store/` - Data access layer with interfaces and PostgreSQL implementation
- `internal/model/` - Database models for transactions, swap requests, and treasury data
- `migrations/schema/` - Database migration files

**Business Logic:**
- `internal/oracle/` - Price oracle and financial calculations
- `internal/telemetry/` - Background transaction indexing and processing
- `internal/btcrpc/` - Bitcoin blockchain interaction via Blockstream API
- `internal/baserpc/` - Ethereum/Base blockchain interaction

**API Layer:**
- `internal/transport/http/` - HTTP server setup with Gin framework
- `internal/handler/` - Request handlers organized by feature (oracle, swap, transaction)
- API routes are organized under `/api/v1/` with middleware for CORS and API key authentication

**Configuration:**
- `internal/utils/config/` - Environment-based configuration with Vault support for production
- `internal/utils/logger/` - Structured logging with Zap
- Uses `.env` files for local development, HashiCorp Vault for production secrets

### Key Features
- BTC ↔ ICY token swap functionality
- Real-time price oracle integration
- Background transaction monitoring and processing (every 2 minutes by default)
- Swagger API documentation
- Production-ready with API key authentication and CORS support

### Environment Configuration
- Local development: Uses `.env` files
- Production: Integrates with HashiCorp Vault for secret management
- Required environment variables documented in README.md

### Background Processing
The service runs scheduled tasks via cron jobs:
- Index BTC transactions from blockchain
- Index ICY transactions from Base chain
- Process pending swap requests
- Default interval: 2 minutes (configurable via `INDEX_INTERVAL`)

## Project Conventions

- Uses Go 1.23+ with modules
- Follows clean architecture patterns with clear separation of concerns
- Database interactions through GORM with PostgreSQL
- Interface-driven design for testability
- Structured logging throughout the application
- Environment-specific configuration management