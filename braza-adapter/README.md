# Checker — Braza Adapter

The **Braza Adapter** is a Go-based microservice that integrates the Checker trading platform with the **Braza Bank FXCore API**.
It fetches balances, retrieves product metadata, creates and executes quotes, and normalizes data into Checker's canonical event formats.
This service runs as part of the low-latency trading stack, communicating with Redis, Postgres, and NATS JetStream.

---

## Features

- **Balance Polling:** Periodically queries Braza `/balance` and publishes normalized balance updates.
- **Quote Lifecycle:**
    - Create a quote (preview quotation).
    - Execute a quote (execute-order).
    - Poll trade status until completion.
- **Product Sync:** Retrieves `/product/list` and persists available instrument products to Postgres.
- **Canonical Mapping:** Translates Braza's payloads into Checker's canonical domain models.
- **Fiber REST API:** Exposes internal endpoints for integration testing and manual operations.
- **Prometheus Metrics:** `/metrics` endpoint exposes runtime metrics.
- **Health Check:** `/health` endpoint returns `"ok"` for probes.

---

## Architecture Overview

```
+---------------------+
|   Checker Core      |
| (Quote/Trade Svc)   |
+----------+----------+
           |
           | NATS (evt.balance.updated.v1, evt.quote.created.v1)
           v
+----------------------------+
|        Braza Adapter       |
|  - Poller (balances)       |
|  - Product Sync            |
|  - REST API (Fiber)        |
|  - Publisher (NATS JS)     |
|  - Mapper (Braza→Canonical)|
+------------+---------------+
             |
             v
  Braza Sandbox / FXCore API
```

---

## Configuration

### Environment Variables

| Name | Description | Default |
|------|--------------|----------|
| `BRAZA_BASE_URL` | Braza API base URL | `https://sandbox.fxcore.brazabank.com.br:8443` |
| `LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |
| `POLL_INTERVAL_SEC` | Interval (seconds) for balance polling | `30` |
| `NATS_URL` | NATS JetStream connection URL | `nats://nats:4222` |
| `POSTGRES_DSN` | Postgres DSN string | *(required)* |
| `AWS_REGION` | Region for Secrets Manager | `us-east-1` |
| `PROM_ADDR` | Prometheus metrics endpoint | `:9090` |

---

## Local Development

### 1. Clone & Build

```bash
git clone https://github.com/Checker-Finance/adapters.git
cd adapters/braza-adapter
make build
```

### 2. Run Locally with Docker Compose

```bash
make up
make run
```

This starts NATS, Postgres, and Redis.

---

## REST API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (`ok`) |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/balances/:client_id` | Retrieve balances from DB |
| `POST` | `/api/v1/quote` | Create quote (Braza preview) |
| `POST` | `/api/v1/quote/:id/execute` | Execute quote |
| `GET` | `/api/v1/products` | List Braza products |

---

## Docker

### Build
```bash
docker build -t checker/braza-adapter:latest .
```

### Run
```bash
docker run -d \
  -e BRAZA_BASE_URL=https://sandbox.fxcore.brazabank.com.br:8443 \
  -e POSTGRES_DSN=postgres://user:pass@host:5432/checker \
  -e NATS_URL=nats://nats:4222 \
  -p 8080:8080 \
  checker/braza-adapter:latest
```

---

## Metrics

Prometheus endpoint at `/metrics` exposes:

- `braza_api_requests_total`
- `nats_messages_total`
- `adapter_errors_total`
- `adapter_last_poll_timestamp`

---

## Design Notes

- **Canonical First:** Canonical models (`model.Quote`, `model.Trade`, etc.) are the source of truth.
- **Mapper Layer:** Handles translation between Braza and Checker schema.
- **Expiry Logic:** The adapter enforces quote TTL only for execution validation, not event emission.
- **Error Propagation:** Upstream errors from Braza are logged and passed through HTTP responses.

---

## Healthcheck Example

```bash
curl -s http://localhost:8080/health
# ok
```

---

## Makefile Commands

| Command | Description |
|----------|--------------|
| `make build` | Compile binary |
| `make run` | Run locally |
| `make lint` | Run linter |
| `make test` | Run tests |
| `make docker-build` | Build Docker image |

---

## CI

- Workflow: `.github/workflows/build-and-push-braza-adapter.yml` (on push to `main` under `braza-adapter/**`).
- Set GitHub secret **`ECR_REPOSITORY_BRAZA`** (e.g. `braza-adapter`) and ensure the ECR repository exists. The same OIDC role (`AWS_ROLE_ARN`) is used as for rio-adapter.

---

## Kubernetes / ArgoCD

- **Paths:** `braza-adapter/k8s/overlays/dev`, `braza-adapter/k8s/overlays/prod`.
- **Namespaces:** `braza-adapter-dev`, `braza-adapter-prod`.
- **Secrets:** ExternalSecrets expect AWS Secrets Manager keys:
  - `braza-adapter/dev` and `braza-adapter/prod` with at least: `DATABASE_URL`, `BRAZA_API_KEY`, `REDIS_PASS`.

---

## License

© 2025 **Checker Corp** — Proprietary and Confidential.

All rights reserved. Redistribution prohibited without written consent.

See [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.
