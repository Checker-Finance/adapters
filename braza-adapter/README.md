# Checker — Braza Adapter

The **Braza Adapter** integrates the Checker trading platform with the **Braza Bank FXCore API**.
It creates and executes quotes, polls trade status, fetches balances, and publishes canonical trade events to NATS JetStream.

## Architecture

```
Checker Core ──NATS──▶ Braza Adapter ──HTTP──▶ Braza FXCore API
                             │
                        NATS JetStream (evt.trade.*.v1.BRAZA)
                             │
                        Redis + Postgres
```

**Status tracking:** Polling only (no webhook support). Default poll interval: 5 minutes.

## HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS + store status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List available products |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ |
| `POST` | `/api/v1/orders` | Execute order |
| `POST` | `/api/v1/resolve-order/:quoteId` | Resolve/finalize order |

## NATS Subjects

| Direction | Subject |
|-----------|---------|
| Inbound (quote request) | `cmd.lp.quote_request.v1.BRAZA` |
| Outbound (interim) | `evt.trade.status_changed.v1.BRAZA` |
| Outbound (final) | `evt.trade.filled.v1.BRAZA` |
| Outbound (final) | `evt.trade.rejected.v1.BRAZA` |
| Outbound (final) | `evt.trade.cancelled.v1.BRAZA` |
| Outbound (final) | `evt.trade.refunded.v1.BRAZA` |

## Configuration

Service-level infrastructure config is loaded from env vars, then overlaid from the AWS Secrets Manager secret at `{ENV}/braza-adapter`.

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `dev` | Deployment environment (`dev`, `uat`, `prod`) |
| `DATABASE_URL` | postgres://checker:checker@localhost/db_checker | Postgres DSN |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream URL |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL |
| `AWS_REGION` | `us-east-2` | AWS region for Secrets Manager |
| `BRAZA_PORT` | `9020` | HTTP server port |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `POLL_INTERVAL` | `5m` | Order status polling interval |
| `CACHE_TTL` | `24h` | Per-client secret cache TTL |
| `CLIENT_BALANCES_IDS` | _(empty)_ | Comma-separated client IDs for balance polling |
| `SETTLEMENT_CUT_OFF` | `17:00` | Settlement cut-off time (local) |

**Per-client secrets** are resolved from AWS Secrets Manager at `{env}/{clientId}/braza` and contain:
`api_key`, `base_url`, `webhook_secret` (and optionally `webhook_sig_header`).

## Local Development

```bash
make up       # Start NATS + Redis via Docker Compose
make build    # Compile binary to ./bin/braza-adapter
make run      # Run locally
make test     # Run all tests with race detector
make lint     # Run golangci-lint
```

## Docker

```bash
# Build (from repo root as build context)
make docker-build

# Run
docker run -d \
  -e ENV=dev \
  -e AWS_REGION=us-east-2 \
  -p 9020:9020 \
  checker/braza-adapter:latest
```

At runtime only `ENV` and `AWS_REGION` are required — all infrastructure URLs are fetched from AWS Secrets Manager at `{env}/braza-adapter`.

## Kubernetes / ArgoCD

- Overlays: `braza-adapter/k8s/overlays/dev`, `braza-adapter/k8s/overlays/prod`
- Secrets injected via External Secrets Operator from `prod/braza-adapter` / `dev/braza-adapter` in AWS Secrets Manager

## CI

Workflow: `.github/workflows/build-and-push-braza-adapter.yml` — triggered on push to `main` under `braza-adapter/**`, `pkg/**`, `internal/**`.

---

© 2025 **Checker Corp** — Proprietary and Confidential.
