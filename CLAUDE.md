# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Structure

This is a monorepo of venue adapters — Go microservices that integrate Checker with external trading venues, normalize data to canonical models, and publish events downstream.

All adapters share a single root Go module (`github.com/Checker-Finance/adapters`).

```
adapters/                    # Root module (go.mod here)
├── pkg/                     # Shared libraries (canonical models, secrets, logger, utils)
│   ├── model/               # Canonical domain models (Quote, Trade, Settlement, etc.)
│   │                        #   + status constants (StatusFilled etc.) + IsTerminal()
│   ├── secrets/             # Generic TTL cache + AWS Secrets Manager provider
│   ├── logger/              # Structured logging (slog)
│   └── utils/               # Utilities (DSN masking, etc.)
├── internal/                # Shared internal packages
│   ├── store/               # Hybrid Redis-first, Postgres-backed persistence layer
│   ├── publisher/           # NATS JetStream event publishing
│   ├── legacy/              # Backward-compatibility trade sync writer + RFQ sweeper
│   ├── rate/                # Rate limiter for venue API calls
│   ├── metrics/             # Shared Prometheus metrics
│   ├── secrets/             # Generic AWSResolver[T any] for multi-tenant config
│   ├── nats/                # Shared NATS command consumer (used by XFX, Zodia, Capa)
│   ├── webhooks/            # Shared HMAC-SHA256 webhook signature validation
│   └── jobs/                # Background jobs (summary refresher)
├── rio-adapter/             # Rio Bank FXCore integration (Fiber + NATS + Postgres/Redis)
├── braza-adapter/           # Braza FX integration (Fiber + NATS + Postgres/Redis)
├── xfx-adapter/             # XFX Trading API integration (Fiber + NATS + Postgres/Redis)
├── zodia-adapter/           # Zodia Markets integration (Fiber + NATS + Postgres/Redis)
├── capa-adapter/            # Capa ramp integration (Fiber + NATS + Postgres/Redis)
├── b2c2-adapter/            # B2C2 Markets integration (NATS only; FOK sync orders)
├── kiiex-adapter/           # Kiiex/AlphaPoint integration (NATS + AlphaPoint WebSocket)
├── docs/                    # Reference documentation (adapters.md)
└── scripts/                 # OIDC AWS setup scripts
```

Each adapter directory has its own `Makefile` and `Dockerfile`. Rio, Braza, XFX, Zodia, and Capa have `k8s/` and `pkg/config/`. Kiiex has `configs/` (symbol mapping JSON) and `pkg/eventbus/` instead.
Docker images are built from the repo root as build context.

## Common Commands

Run from within an adapter directory (e.g., `rio-adapter/`, `braza-adapter/`, or `kiiex-adapter/`):

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

Rio, Braza, XFX, Zodia, and Capa follow this layered structure (Fiber + NATS + Postgres/Redis):

```
<adapter>/
  cmd/<adapter>/main.go  # Entry point: config, DI wiring, server start, graceful shutdown
  internal/
    <venue>/
      service.go         # Business logic: quote creation, order execution, status tracking
      client.go          # HTTP client for venue API with rate limiting
      poller.go          # Scheduled polling for order status (fallback if webhooks fail)
      webhook_handler.go # Real-time order update callbacks (signature-validated)
      command_consumer.go # Thin wrapper around internal/nats.CommandConsumer
    api/                 # Fiber REST endpoints (handlers, routes, middleware)
    auth/                # Venue-specific auth (JWT management, token caching)
    secrets/             # Thin wrapper around internal/secrets.AWSResolver[T]
    metrics/             # Venue-specific Prometheus metrics
  pkg/config/config.go   # Environment variable configuration loader
  Dockerfile             # Multi-stage build; uses repo root as build context
  Makefile               # All targets delegate to repo root via `cd ..`
  k8s/                   # Kustomize base + dev/prod overlays
```

**XFX-specific notes:**
- Auth: OAuth2 Client Credentials via Auth0; `auth0_endpoint` and `auth0_audience` come from the **per-client** secret `{env}/{clientId}/xfx` — no env vars needed for auth configuration
- Per-client secrets: `{env}/{clientId}/xfx` → `{"client_id", "client_secret", "base_url", "auth0_endpoint", "auth0_audience"}`
- No webhooks — polling only (default `XFX_POLL_INTERVAL=15s`)
- NATS subjects: `evt.trade.<status>.v1.XFX`
- HTTP port: 9030

**XFX API endpoints used:**
- `POST /v1/customer/quotes` — Request executable quote (15s validity window)
- `GET /v1/customer/quotes/{quoteId}` — Get quote status
- `POST /v1/customer/quotes/{quoteId}/execute` — Execute trade → creates transaction
- `GET /v1/customer/transactions/{transactionId}` — Poll transaction status

**Supported currency pairs:** USD/MXN, USDT/MXN, USDC/MXN, USD/COP, USDT/COP, USDC/COP, USD/USDT, USD/USDC (all min $100,000 USD)

Kiiex follows a different shape (NATS + AlphaPoint WebSocket; no Fiber/Postgres/Redis):

```
kiiex-adapter/
  cmd/kiiex-adapter/main.go   # Entry point: config, auth, DI wiring, graceful shutdown
  internal/
    alphapoint/               # WebSocket client + session for AlphaPoint/Kiiex exchange
    config/                   # Environment variable config loader
    instruments/              # Symbol mapping (loaded from configs/symbol_mapping.json)
    nats/                     # NATS command consumer + publisher
    order/                    # Order models, commands, events, service, adapters
    security/                 # Auth (HMAC signature) + AWS Secrets Manager fetch
    tracking/                 # Trade status service (subscribes to eventbus, publishes results)
  pkg/eventbus/               # In-process pub/sub event bus (kiiex-specific)
  configs/                    # symbol_mapping.json
  Dockerfile                  # Multi-stage build; uses repo root as build context
  Makefile                    # All targets delegate to repo root via `cd ..`
```

B2C2 is NATS-only (no Fiber/Postgres/Redis/webhooks), using FOK synchronous orders:

```
b2c2-adapter/
  cmd/b2c2-adapter/main.go
  internal/
    b2c2/        # Service, client, mapper, types
    nats/        # Command consumer + publisher
    secrets/     # AWSResolver[B2C2ClientConfig] wrapper
  pkg/config/config.go
```

### Key Design Patterns

- **Single root module** — `github.com/Checker-Finance/adapters`; shared packages live in `pkg/` and `internal/`
- **Multi-tenant by design** — every operation keyed by `clientID`
- **Generic secret resolver** — `internal/secrets.AWSResolver[T any]` resolves per-client config from AWS Secrets Manager; all Fiber+NATS adapters wrap it in `<adapter>/internal/secrets/`
- **Per-client secrets** — resolved at `{env}/{clientId}/{venue}`, cached in-memory with TTL
- **Shared NATS command consumer** — `internal/nats.CommandConsumer` handles subscribe → unmarshal → timeout → dispatch; XFX, Zodia, Capa wrap it with a thin `CommandConsumer` struct in their own package
- **Shared webhook validation** — `internal/webhooks.ValidateHMACSHA256(secret, signature, body)` used by Rio, Zodia, Capa webhook handlers
- **Canonical status constants** — `pkg/model`: `StatusFilled`, `StatusCancelled`, `StatusRejected`, `StatusPending`, `StatusRefunded` + `IsTerminal(status string) bool`; never redefine per-file
- **Dual order-status mechanism** (Rio, Zodia, Capa) — webhook handler for real-time updates + poller as fallback
- **Canonical event envelopes** (Fiber+NATS adapters) — all NATS events wrapped with correlation IDs and metadata
- **Hybrid storage** (Fiber+NATS adapters) — Redis for speed, Postgres for durability; store layer abstracts both
- **In-process event bus** (Kiiex) — `pkg/eventbus` decouples order service from NATS publisher and trade status tracker
- **Dependency injection** via constructor functions; interface-based abstractions for testability

### NATS Event Subjects

Published to JetStream with format `evt.trade.<event>.v1.<VENUE>`. Examples:
- `evt.trade.status_changed.v1.RIO`
- `evt.trade.filled.v1.RIO`
- `evt.trade.rejected.v1.RIO`
- `evt.trade.cancelled.v1.RIO`
- `evt.trade.refunded.v1.RIO`

Each adapter uses its own venue suffix (BRAZA, XFX, ZODIA, CAPA, B2C2, KIIEX).

### NATS (Kiiex)

Kiiex uses NATS JetStream (migrated from RabbitMQ):
- **Consumer** (`internal/nats/command_consumer.go`) — subscribes to `cmd.lp.trade_execute.v1.KIIEX` and `cmd.lp.trade_cancel.v1.KIIEX`
- **Publisher** (`internal/nats/publisher.go`) — subscribes to in-process eventbus, publishes `evt.trade.filled.v1.KIIEX` / `evt.trade.cancelled.v1.KIIEX`
- In-process flow: NATS consumer → order service → AlphaPoint WebSocket → eventbus → NATS publisher

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
1. Live under `<name>-adapter/` with the same directory layout as the closest existing adapter
2. Import shared packages from `github.com/Checker-Finance/adapters/pkg/...` and `github.com/Checker-Finance/adapters/internal/...`
3. For Fiber + NATS adapters (rio/braza pattern): wrap `internal/secrets.AWSResolver[T]` with a typed facade in `<adapter>/internal/secrets/` and use `internal/legacy.NewTradeSyncWriter(pool, "<adapter-name>")` for trade syncing
4. Use `internal/nats.CommandConsumer` for NATS command subscriptions (wrap with a thin local struct)
5. Use `internal/webhooks.ValidateHMACSHA256` for webhook signature validation
6. Use `model.IsTerminal(status)` — do not define a local `isTerminalStatus` function
7. Mirror Makefile targets and Dockerfile shape (root build context) from an existing adapter
8. Add a CI workflow under `.github/workflows/build-and-push-<name>-adapter.yml` with path triggers for `<name>-adapter/**`, `pkg/**`, `internal/**`, `go.mod`, `go.sum`
