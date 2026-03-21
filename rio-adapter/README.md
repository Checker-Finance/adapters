# Checker — Rio Adapter

The **Rio Adapter** integrates the Checker trading platform with **Rio Bank's FXCore API**.
It creates and executes quotes, tracks order status via webhooks and polling, and publishes canonical trade events to NATS JetStream.

## Architecture

```
Checker Core ──NATS──▶ Rio Adapter ──HTTP──▶ Rio Trading API
                            │
                       NATS JetStream (evt.trade.*.v1.RIO)
                            │
                       Redis + Postgres
```

**Status tracking:** Webhook callbacks (primary) + polling fallback (`RIO_POLL_INTERVAL`, default 30s).

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
| `POST` | `/webhooks/rio/orders` | Rio webhook callback (signature-validated via `X-Rio-Signature`) |

## NATS Subjects

| Direction | Subject |
|-----------|---------|
| Inbound (quote request) | `cmd.lp.quote_request.v1.RIO` |
| Outbound (interim) | `evt.trade.status_changed.v1.RIO` |
| Outbound (final) | `evt.trade.filled.v1.RIO` |
| Outbound (final) | `evt.trade.rejected.v1.RIO` |
| Outbound (final) | `evt.trade.cancelled.v1.RIO` |
| Outbound (final) | `evt.trade.refunded.v1.RIO` |

## Configuration

Service-level infrastructure config is loaded from env vars, then overlaid from the AWS Secrets Manager secret at `{ENV}/rio-adapter`.

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `dev` | Deployment environment (`dev`, `uat`, `prod`) |
| `DATABASE_URL` | postgres://checker:checker@localhost/db_checker | Postgres DSN |
| `NATS_URL` | `nats://localhost:4222` | NATS JetStream URL |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL |
| `AWS_REGION` | `us-east-2` | AWS region for Secrets Manager |
| `RIO_PORT` | `9010` | HTTP server port |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `RIO_POLL_INTERVAL` | `30s` | Order status polling interval (webhook fallback) |
| `POLL_INTERVAL` | `5m` | General polling interval |
| `CACHE_TTL` | `24h` | Per-client secret cache TTL |
| `SETTLEMENT_CUT_OFF` | `17:00` | Settlement cut-off time (local) |

**Per-client secrets** are resolved from AWS Secrets Manager at `{env}/{clientId}/rio` and contain:
`api_key`, `base_url`, `country`, `webhook_url`, `webhook_secret`, `webhook_sig_header`.

## Status Normalization

Rio has 42+ order statuses, normalized to canonical values:

| Rio Status | Canonical |
|------------|-----------|
| `created` | `pending` |
| `processing`, `sourcingLiquidity`, `verifying` | `submitted` |
| `paid`, `filled`, `complete`, `settled` | `filled` |
| `cancelled`, `expired`, `timeout` | `cancelled` |
| `failed`, `rejected`, `payment_failed` | `rejected` |
| `refunded`, `refund_complete` | `refunded` |

## Local Development

```bash
make up       # Start NATS + Redis via Docker Compose
make build    # Compile binary to ./bin/rio-adapter
make run      # Run locally
make test     # Run all tests with race detector
make integration-test  # Run integration tests against Rio sandbox
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
  -p 9010:9010 \
  checker/rio-adapter:latest
```

At runtime only `ENV` and `AWS_REGION` are required — all infrastructure URLs are fetched from AWS Secrets Manager at `{env}/rio-adapter`.

## Kubernetes / ArgoCD

- Overlays: `rio-adapter/k8s/overlays/dev`, `rio-adapter/k8s/overlays/prod`
- Secrets injected via External Secrets Operator from `prod/rio-adapter` / `dev/rio-adapter` in AWS Secrets Manager

## CI

Workflow: `.github/workflows/build-and-push-rio-adapter.yml` — triggered on push to `main` under `rio-adapter/**`, `pkg/**`, `internal/**`.

---

© 2025 **Checker Corp** — Proprietary and Confidential.
