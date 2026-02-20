# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Structure

This is a monorepo of venue adapters — Go microservices that integrate Checker with external trading venues, normalize data to canonical models, and publish events to NATS JetStream.

```
adapters/
├── rio-adapter/     # Rio Bank FXCore integration (production)
├── braza-adapter/   # Stub/scaffolding (replacement pending)
└── scripts/         # OIDC AWS setup scripts
```

Each adapter is an independent Go module with its own `go.mod`, `Makefile`, `Dockerfile`, and `k8s/` directory.

## Common Commands

Run from within the adapter directory (e.g., `rio-adapter/` or `braza-adapter/`):

```bash
make build              # Compile binary to ./bin/<adapter-name>
make run                # Run service locally
make test               # Run all tests with race detector
make integration-test   # Run integration tests against live sandbox (rio-adapter only)
make bench              # Run benchmarks
make cover              # Generate HTML coverage report
make fmt                # Format code (go fmt)
make lint               # Run golangci-lint (5m timeout)
make up                 # Start NATS + Redis via Docker Compose
make down               # Stop containers
make docker-build       # Build Docker image
make bump-patch         # Bump patch version in VERSION file
make bump-minor         # Bump minor version
make bump-major         # Bump major version
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
cmd/main.go              # Entry point: config, DI wiring, server start, graceful shutdown
internal/
  <venue>/
    service.go           # Business logic: quote creation, order execution, status tracking
    client.go            # HTTP client for venue API with rate limiting
    poller.go            # Scheduled polling for order status (fallback if webhooks fail)
    webhook_handler.go   # Real-time order update callbacks (signature-validated)
  api/                   # Fiber REST endpoints (handlers, routes, middleware)
  publisher/             # NATS JetStream event publishing
  store/                 # Hybrid Redis-first, Postgres-backed persistence layer
  secrets/               # Per-client config resolved from AWS Secrets Manager
  rate/                  # Rate limiter for venue API calls
  metrics/               # Prometheus metrics
  legacy/                # Backward-compatibility trade sync writer
pkg/
  config/config.go       # Environment variable configuration loader
  logger/                # Structured logging (zap)
  model/                 # Canonical domain models (Quote, Trade, Settlement, etc.)
  secrets/               # AWS SDK wrapper
migrations/              # SQL migrations (Postgres)
k8s/                     # Kustomize base + dev/prod overlays
```

### Key Design Patterns

- **Multi-tenant by design** — every operation keyed by `clientID`
- **Per-client secrets** — API keys/URLs resolved from AWS Secrets Manager at `{env}/{adapter}/{clientId}`, cached in-memory with TTL
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
- Each adapter has its own workflow triggered on push to `main` with path filters (`rio-adapter/**`, `braza-adapter/**`) and manual dispatch
- Pipeline: `go test -race -count=1 ./...` + `golangci-lint` → Docker build → Trivy vulnerability scan → push to ECR
- OIDC authentication — no long-lived AWS keys; IAM role assumed via GitHub Actions OIDC

## Deployment

- Kubernetes via ArgoCD + Kustomize overlays (`k8s/overlays/dev`, `k8s/overlays/prod`)
- Secrets injected via External Secrets Operator (fetches from AWS Secrets Manager)
- Version tracked in `VERSION` file; Docker images tagged with short git SHA + `:latest`

## Adding a New Adapter

See `braza-adapter/README.md` for the scaffolding pattern. New adapters should mirror the rio-adapter structure: same Makefile targets, Dockerfile shape, k8s layout, and canonical model usage from `pkg/model/`.
