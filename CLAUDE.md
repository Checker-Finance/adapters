# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Structure

This is a monorepo of venue adapters — Go microservices that integrate Checker with external trading venues, normalize data to canonical models, and publish events to NATS JetStream.

All adapters share a single root Go module (`github.com/Checker-Finance/adapters`).

```
adapters/                    # Root module (go.mod here)
├── pkg/                     # Shared libraries (canonical models, secrets, logger, utils)
│   ├── model/               # Canonical domain models (Quote, Trade, Settlement, etc.)
│   ├── secrets/             # Generic TTL cache + AWS Secrets Manager provider
│   ├── logger/              # Structured logging (zap)
│   └── utils/               # Utilities (DSN masking, etc.)
├── internal/                # Shared internal packages
│   ├── store/               # Hybrid Redis-first, Postgres-backed persistence layer
│   ├── publisher/           # NATS JetStream event publishing
│   ├── legacy/              # Backward-compatibility trade sync writer + RFQ sweeper
│   ├── rate/                # Rate limiter for venue API calls
│   ├── metrics/             # Shared Prometheus metrics
│   ├── secrets/             # Generic AWSResolver[T any] for multi-tenant config
│   └── jobs/                # Background jobs (summary refresher)
├── rio-adapter/             # Rio Bank FXCore integration (production)
├── braza-adapter/           # Braza FX integration
└── scripts/                 # OIDC AWS setup scripts
```

Each adapter directory has its own `Makefile`, `Dockerfile`, `k8s/`, and `pkg/config/`.
Docker images are built from the repo root as build context.

## Common Commands

Run from within an adapter directory (e.g., `rio-adapter/` or `braza-adapter/`):

```bash
make build              # Compile binary to ./bin/<adapter-name>
make run                # Run service locally
make test               # Run all tests with race detector (root module scope)
make integration-test   # Run integration tests against live sandbox (rio-adapter only)
make bench              # Run benchmarks
make cover              # Generate HTML coverage report
make fmt                # Format code (go fmt)
make lint               # Run golangci-lint (5m timeout)
make up                 # Start NATS + Redis via Docker Compose
make down               # Stop containers
make docker-build       # Build Docker image (from repo root context)
make bump-patch         # Bump patch version in VERSION file
make bump-minor         # Bump minor version
make bump-major         # Bump major version
```

Run tests from repo root:
```bash
go test -race -count=1 ./...
```

Run a single test:
```bash
go test -v -run TestFunctionName ./path/to/package
```

Integration tests require build tag:
```bash
go test -v -tags integration -run TestName ./...
```

## Architecture

### Adapter Pattern

Each adapter follows this layered structure:

```
<adapter>/
  cmd/<adapter>/main.go  # Entry point: config, DI wiring, server start, graceful shutdown
  internal/
    <venue>/
      service.go         # Business logic: quote creation, order execution, status tracking
      client.go          # HTTP client for venue API with rate limiting
      poller.go          # Scheduled polling for order status (fallback if webhooks fail)
      webhook_handler.go # Real-time order update callbacks (signature-validated)
    api/                 # Fiber REST endpoints (handlers, routes, middleware)
    auth/                # Venue-specific auth (JWT management, token caching)
    secrets/             # Thin wrapper around internal/secrets.AWSResolver[T]
    metrics/             # Venue-specific Prometheus metrics
  pkg/config/config.go   # Environment variable configuration loader
  Dockerfile             # Multi-stage build; uses repo root as build context
  Makefile               # All targets delegate to repo root via `cd ..`
  k8s/                   # Kustomize base + dev/prod overlays
```

### Key Design Patterns

- **Single root module** — `github.com/Checker-Finance/adapters`; shared packages live in `pkg/` and `internal/`
- **Multi-tenant by design** — every operation keyed by `clientID`
- **Generic secret resolver** — `internal/secrets.AWSResolver[T any]` resolves per-client config from AWS Secrets Manager; each adapter wraps it with a thin, typed facade in `<adapter>/internal/secrets/`
- **Per-client secrets** — resolved at `{env}/{clientId}/{venue}`, cached in-memory with TTL
- **Dual order-status mechanism** — webhook handler for real-time updates + poller as fallback
- **Canonical event envelopes** — all NATS events wrapped with correlation IDs and metadata
- **Hybrid storage** — Redis for speed, Postgres for durability; store layer abstracts both
- **Dependency injection** via constructor functions; interface-based abstractions for testability

### NATS Event Subjects (Rio)

Published to JetStream with format `evt.trade.<event>.v1.RIO`:
- `evt.trade.status_changed.v1.RIO`
- `evt.trade.filled.v1.RIO`
- `evt.trade.rejected.v1.RIO`
- `evt.trade.cancelled.v1.RIO`
- `evt.trade.refunded.v1.RIO`

## CI/CD

Workflows in `.github/workflows/`:
- Each adapter has its own workflow triggered on push to `main` with path filters covering both the adapter directory and shared packages (`pkg/**`, `internal/**`, `go.mod`, `go.sum`)
- Pipeline: `go test -race -count=1 ./...` + `golangci-lint` → Docker build → Trivy vulnerability scan → push to ECR
- OIDC authentication — no long-lived AWS keys; IAM role assumed via GitHub Actions OIDC

## Deployment

- Kubernetes via ArgoCD + Kustomize overlays (`k8s/overlays/dev`, `k8s/overlays/prod`)
- Secrets injected via External Secrets Operator (fetches from AWS Secrets Manager)
- Version tracked in `VERSION` file; Docker images tagged with short git SHA + `:latest`

## Adding a New Adapter

New adapters should:
1. Live under `<name>-adapter/` with the same directory layout as existing adapters
2. Import shared packages from `github.com/Checker-Finance/adapters/pkg/...` and `github.com/Checker-Finance/adapters/internal/...`
3. Wrap `internal/secrets.AWSResolver[T]` with a typed facade in `<adapter>/internal/secrets/`
4. Use `internal/legacy.NewTradeSyncWriter(pool, logger, "<adapter-name>")` for trade syncing
5. Mirror Makefile targets, Dockerfile shape (root build context), k8s layout, and CI workflow
