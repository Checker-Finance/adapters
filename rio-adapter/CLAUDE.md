# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
make build              # Compile binary to ./bin/rio-adapter
make run                # Run service locally
make test               # Run all tests with race detector
make integration-test   # Run integration tests against Rio sandbox
make bench              # Run benchmarks
make cover              # Generate coverage report
make fmt                # Format code
make lint               # Run golangci-lint (5m timeout)
make up                 # Start NATS + Redis containers
make down               # Stop containers
make docker-build       # Build Docker image
```

Run a single test:
```bash
go test -v -run TestFunctionName ./path/to/package
```

Integration tests require the `integration` build tag:
```bash
go test -v -tags integration -run TestName ./...
```

## Architecture Overview

Rio Adapter is a Go microservice that integrates Checker with Rio Bank's FXCore API. It handles quote creation/execution, tracks order status via webhooks and polling, and publishes trade events to NATS JetStream.

### Core Components

- **Service** (`internal/rio/service.go`) — Orchestrates quote creation, order execution, and status tracking
- **Client** (`internal/rio/client.go`) — Rio API HTTP client with rate limiting
- **Poller** (`internal/rio/poller.go`) — Scheduled order status polling (fallback when webhooks are unavailable)
- **Webhook Handler** (`internal/rio/webhook_handler.go`) — Real-time order update callbacks; validates `X-Rio-Signature` header using shared `internal/webhooks.ValidateHMACSHA256`
- **REST API** (`internal/api/`) — Fiber-based HTTP server on port 9010
- **Publisher** (`internal/publisher/`) — NATS JetStream event publishing
- **Secrets Manager** (`internal/secrets/`) — Per-client config resolution from AWS Secrets Manager, cached with TTL
- **Hybrid Store** (`internal/store/`) — Redis-first, Postgres-backed persistence

### Data Flow

1. Checker Core calls REST endpoints or sends NATS commands to create quotes and execute orders
2. Service calls Rio Bank API via Client; per-client credentials fetched from AWS Secrets Manager
3. Order status updates arrive via Rio webhook callbacks (primary) or Poller (fallback)
4. Normalized events published to NATS JetStream under `evt.trade.<event>.v1.RIO`
5. State cached in Redis and persisted to Postgres via the hybrid Store

### Key Patterns

- All events wrapped in canonical `Envelope` with correlation IDs
- Multi-tenant: every operation keyed by `clientID`; per-client secrets at `{env}/{clientId}/rio`
- Dependency injection via constructors; interface-based abstractions for testability
- Dual status-tracking: webhook handler + poller as fallback
- Status constants: use `model.StatusFilled`, `model.StatusCancelled`, etc. from `pkg/model/status.go`
- Terminal status check: use `model.IsTerminal(status)` — do not redefine per-file

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS + store status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List available products |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ |
| `POST` | `/api/v1/orders` | Execute order |
| `POST` | `/api/v1/resolve-order/:quoteId` | Resolve/finalize order |
| `POST` | `/webhooks/rio/orders` | Rio webhook callback (signature-validated) |

## Configuration

Key environment variables; loaded in `pkg/config/config.go`:

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `dev` | Deployment environment |
| `DATABASE_URL` | postgres://checker:checker@localhost/db_checker | Postgres DSN |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream |
| `REDIS_URL` | `redis://localhost:6379` | Redis |
| `AWS_REGION` | `us-east-2` | For Secrets Manager |
| `RIO_PORT` | `9010` | HTTP server port |
| `RIO_POLL_INTERVAL` | `30s` | Order status polling interval |
| `LOG_LEVEL` | `info` | Log level |

Per-client secrets resolved from AWS Secrets Manager at `{env}/{clientId}/rio`: `api_key`, `base_url`, `country`, `webhook_url`, `webhook_secret`, `webhook_sig_header`.

## Database

Migrations are in `migrations/` and manage the Postgres schema for trade persistence, balance snapshots, and the product catalog.
