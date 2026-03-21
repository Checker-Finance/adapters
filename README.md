# Checker Adapters

Monorepo of venue adapters — Go microservices that integrate Checker with external trading venues, normalize data to canonical models, and publish events to NATS JetStream.

All adapters share a single root Go module (`github.com/Checker-Finance/adapters`). Shared libraries live in `pkg/` and `internal/`.

## Adapters

| Adapter | Venue | Port | Description |
|---------|-------|------|-------------|
| [rio-adapter](./rio-adapter/) | Rio Bank FXCore | 9010 | REST + NATS + Postgres/Redis; webhooks + polling |
| [braza-adapter](./braza-adapter/) | Braza FX | 9020 | REST + NATS + Postgres/Redis; polling only |
| [xfx-adapter](./xfx-adapter/) | XFX Trading | 9030 | REST + NATS + Postgres/Redis; OAuth2/Auth0; polling only |
| [zodia-adapter](./zodia-adapter/) | Zodia Markets | 9040 | REST + NATS + Postgres/Redis; HMAC; webhooks + polling |
| [kiiex-adapter](./kiiex-adapter/) | Kiiex/AlphaPoint | 9070 | NATS + AlphaPoint WebSocket; no Postgres/Redis |
| [b2c2-adapter](./b2c2-adapter/) | B2C2 Markets | 9050 | NATS; static token; FOK sync orders; no Postgres/Redis |
| [capa-adapter](./capa-adapter/) | Capa (LATAM ramp) | 9060 | REST + NATS + Postgres/Redis; static API key; webhooks + polling; cross/on/off-ramp |

For a full breakdown of HTTP endpoints and NATS subjects for each adapter, see [docs/adapters.md](./docs/adapters.md).

## Repository Layout

```
adapters/
├── go.mod / go.sum          # Single root module
├── pkg/                     # Shared public packages
│   ├── model/               # Canonical domain models + status constants (IsTerminal)
│   ├── secrets/             # Generic TTL cache + AWS Secrets Manager provider
│   ├── logger/              # Structured slog logger
│   └── utils/               # DSN masking, etc.
├── internal/                # Shared internal packages
│   ├── store/               # Hybrid Redis-first, Postgres-backed persistence
│   ├── publisher/           # NATS JetStream event publishing
│   ├── legacy/              # Trade sync writer + RFQ sweeper
│   ├── rate/                # Rate limiter for venue API calls
│   ├── metrics/             # Shared Prometheus metrics
│   ├── secrets/             # Generic AWSResolver[T any]
│   ├── nats/                # Shared NATS command consumer (XFX, Zodia, Capa)
│   ├── webhooks/            # Shared HMAC-SHA256 webhook signature validation
│   └── jobs/                # Background jobs (summary refresher)
├── rio-adapter/
├── braza-adapter/
├── xfx-adapter/
├── zodia-adapter/
├── kiiex-adapter/
├── b2c2-adapter/
├── capa-adapter/
├── docs/                    # Reference documentation
└── scripts/                 # OIDC AWS setup scripts
```

## Development

Run from any adapter directory:

```bash
make build        # Compile binary
make test         # Run all tests with race detector (root module scope)
make lint         # Run golangci-lint
make up           # Start NATS + Redis via Docker Compose
make down         # Stop containers
make docker-build # Build Docker image (repo root as build context)
```

Run all tests from the repo root:

```bash
go test -race -count=1 ./...
```

## CI / Deployment

- **CI:** Each adapter has its own GitHub Actions workflow (`.github/workflows/build-and-push-<name>-adapter.yml`) triggered on push to `main`. Pipeline: tests + lint → Docker build → Trivy scan → push to ECR. Path filters cover the adapter directory and shared packages (`pkg/**`, `internal/**`, `go.mod`, `go.sum`).
- **Auth:** OIDC — no long-lived AWS keys; IAM role assumed via GitHub Actions OIDC.
- **Deployment:** Kubernetes via ArgoCD + Kustomize overlays (`k8s/overlays/dev`, `k8s/overlays/prod`). Secrets injected via External Secrets Operator from AWS Secrets Manager.
- **Versioning:** `VERSION` file per adapter; Docker images tagged with short git SHA + `:latest`.

## History

- **rio-adapter** moved from [Checker-Finance/rio-adapter](https://github.com/Checker-Finance/rio-adapter). Last standalone release: `v1.0.0-pre-commons`.
