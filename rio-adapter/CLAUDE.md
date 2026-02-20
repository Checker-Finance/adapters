# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
make build          # Compile binary to ./bin/rio-adapter
make run            # Run service locally
make test           # Run all tests with race detector
make bench          # Run benchmarks
make cover          # Generate coverage report
make fmt            # Format code
make lint           # Run golangci-lint (5m timeout)
make up             # Start NATS + Redis containers
make down           # Stop containers
make docker-build   # Build Docker image
```

Run a single test:
```bash
go test -v -run TestFunctionName ./path/to/package
```

## Architecture Overview

Rio Adapter is a Go microservice that integrates the Checker trading platform with Rio Bank's FXCore API. It polls balances, handles quote creation/execution, and publishes events to NATS JetStream.

### Core Components

- **Service** (`internal/rio/service.go`) - Orchestrates quote/trade operations with Rio API
- **Poller** (`internal/rio/poller.go`) - Scheduled balance polling and trade status tracking
- **REST API** (`internal/api/`) - Fiber-based HTTP endpoints on port 9010
- **Publisher** (`internal/publisher/`) - NATS JetStream event publishing
- **Auth Manager** (`internal/auth/`) - Multi-tenant JWT management with token caching
- **Hybrid Store** (`internal/store/`) - Redis-first, Postgres-backed caching

### Data Flow

1. Poller fetches balances from Rio API at configurable intervals
2. Data is normalized to canonical models (`pkg/model/`)
3. Events are published to NATS JetStream via Publisher
4. Balances cached in Redis, persisted to Postgres

### Key Patterns

- All events wrapped in canonical `Envelope` format with correlation IDs
- Multi-tenant by design: everything keyed by tenant/client/venue
- Dependency injection via constructor functions
- Interface-based abstractions for testability

## API Endpoints

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `GET /api/v1/balances/:client_id` - Retrieve balances
- `POST /api/v1/quotes` - Create RFQ
- `POST /api/v1/quotes/:quotation_id/execute` - Execute quote
- `GET /api/v1/products` - List products

## Configuration

Key environment variables (see `.env.sample`):
- `DATABASE_URL` - Postgres connection
- `NATS_URL` - NATS JetStream (default: nats://localhost:4222)
- `REDIS_ADDR` - Redis cache (default: localhost:6379)
- `RIO_BASE_URL` - Rio API endpoint
- `CLIENT_BALANCES_IDS` - Comma-separated client IDs to poll
- `POLL_INTERVAL` - Balance polling interval (default: 5m)

Configuration is loaded in `pkg/config/config.go`.

## Database

Migrations are in `/migrations/` and manage:
- Ledger schema for balance events
- Balance snapshots and summaries
- Product catalog (venue_products)
