# Checker — Rio Adapter

The **Rio Adapter** is a Go-based microservice that integrates the Checker trading platform with **Rio's Trading API**.
It creates quotes, executes orders, tracks order status via webhooks and polling, and normalizes data into Checker's canonical event formats.
This service runs as part of the low-latency trading stack, communicating with Redis, Postgres, and NATS JetStream.

---

## Features

- **Quote Creation:** Create quotes via Rio's `/api/quotes` endpoint.
- **Order Execution:** Execute quotes to create orders via `/api/orders`.
- **Status Tracking:**
    - Primary: Webhook callbacks for real-time order status updates.
    - Fallback: Polling for order status when webhooks are unavailable.
- **Status Normalization:** Maps Rio's 42+ order statuses to canonical statuses (pending, submitted, filled, cancelled, rejected, refunded).
- **Canonical Mapping:** Translates Rio's payloads into Checker's canonical domain models.
- **Fiber REST API:** Exposes endpoints for quote creation and execution.
- **NATS Events:** Publishes trade status events to NATS JetStream.
- **Prometheus Metrics:** `/metrics` endpoint exposes runtime metrics.
- **Health Check:** `/health` endpoint returns `"ok"` for probes.

---

## Key Differences from Rio Adapter

| Feature | Rio | Rio |
|---------|-------|-----|
| Authentication | JWT token via `/token` | API Key via `x-api-key` header |
| Balance endpoint | Yes | **No** - Rio has no balance API |
| Quote creation | `POST /preview-quotation` | `POST /api/quotes` |
| Order execution | `POST /execute-order` | `POST /api/orders` with `quoteId` |
| Status tracking | Polling only | **Webhooks + Polling fallback** |
| Status values | ~6 statuses | **42+ statuses** |

---

## Architecture Overview

```
+---------------------+
|   Checker Core      |
| (Quote/Trade Svc)   |
+----------+----------+
           |
           | NATS (evt.trade.*.v1.RIO)
           v
+----------------------------+
|        Rio Adapter         |
|  - REST API (Fiber)        |
|  - Webhook Handler         |
|  - Status Poller           |
|  - Publisher (NATS JS)     |
|  - Mapper (Rio→Canonical)  |
+------------+---------------+
             |
             v
      Rio Trading API
  (app.sandbox.rio.trade)
```

---

## Configuration

### Environment Variables

| Name | Description | Default |
|------|-------------|---------|
| `RIO_BASE_URL` | Rio API base URL | `https://app.sandbox.rio.trade/api` |
| `RIO_API_KEY` | Rio API key | *(required)* |
| `RIO_COUNTRY` | Default country (US, MX, PE) | `US` |
| `RIO_POLL_INTERVAL` | Polling interval for order status | `30s` |
| `RIO_WEBHOOK_URL` | Callback URL for webhooks | *(optional)* |
| `RIO_PORT` | HTTP server port | `9010` |
| `LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |
| `NATS_URL` | NATS JetStream connection URL | `nats://localhost:4222` |
| `DATABASE_URL` | Postgres DSN string | *(required)* |
| `REDIS_ADDR` | Redis address | `localhost:6379` |
| `AWS_REGION` | Region for Secrets Manager | `us-east-2` |

---

## Local Development

### 1. Clone & Build

```bash
git clone https://github.com/Checker-Finance/adapters.git
cd adapters/rio-adapter
make build
```

### 2. Run Locally with Docker Compose

```bash
docker-compose up -d
export RIO_API_KEY=your-api-key
make run
```

This starts NATS, Redis, Postgres, and the Rio Adapter.

---

## REST API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (`ok`) |
| `POST` | `/api/v1/quotes` | Create quote |
| `POST` | `/api/v1/quotes/:quotation_id/execute` | Execute quote |
| `POST` | `/webhooks/rio/orders` | Rio webhook callback |

### Create Quote

```bash
curl -X POST http://localhost:9010/api/v1/quotes \
  -H "Content-Type: application/json" \
  -d '{
    "clientId": "client-123",
    "pair": "usdc/usd",
    "orderSide": "buy",
    "quantity": 1000
  }'
```

### Execute Quote

```bash
curl -X POST http://localhost:9010/api/v1/quotes/{quoteId}/execute \
  -H "Content-Type: application/json" \
  -d '{
    "clientId": "client-123"
  }'
```

---

## NATS Event Subjects

| Subject | Description |
|---------|-------------|
| `evt.trade.status_changed.v1.RIO` | Order status changed |
| `evt.trade.filled.v1.RIO` | Order filled/completed |
| `evt.trade.rejected.v1.RIO` | Order rejected/failed |
| `evt.trade.cancelled.v1.RIO` | Order cancelled |
| `evt.trade.refunded.v1.RIO` | Order refunded |

---

## Status Mapping

Rio has 42+ order statuses. They are normalized to canonical statuses:

| Rio Status | Canonical |
|------------|-----------|
| `created` | `pending` |
| `processing`, `sourcingLiquidity`, `verifying` | `submitted` |
| `paid`, `filled`, `complete`, `settled` | `filled` |
| `cancelled`, `expired`, `timeout` | `cancelled` |
| `failed`, `rejected`, `payment_failed` | `rejected` |
| `refunded`, `refund_complete` | `refunded` |

---

## Docker

### Build
```bash
docker build -t checker/rio-adapter:latest .
```

### Run
```bash
docker run -d \
  -e RIO_BASE_URL=https://app.sandbox.rio.trade/api \
  -e RIO_API_KEY=your-api-key \
  -e DATABASE_URL=postgres://user:pass@host:5432/checker \
  -e NATS_URL=nats://nats:4222 \
  -p 9010:9010 \
  checker/rio-adapter:latest
```

---

## Metrics

Prometheus endpoint at `/metrics` exposes:

- `rio_api_requests_total`
- `rio_api_request_duration_seconds`
- `nats_messages_total`
- `adapter_errors_total`
- `adapter_last_poll_timestamp`

---

## Webhooks

Rio supports webhooks for real-time order status updates. Configure `RIO_WEBHOOK_URL` to enable automatic webhook registration on startup.

The adapter will:
1. Register a webhook with Rio at startup
2. Receive order status updates at `/webhooks/rio/orders`
3. Cancel active polling when webhook delivers status
4. Fall back to polling if webhooks fail

---

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Compile binary to `./bin/rio-adapter` |
| `make run` | Run locally |
| `make test` | Run tests with race detector |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make docker-build` | Build Docker image |

---

## Testing

```bash
# Run all tests
make test

# Run Rio-specific tests
go test -v ./internal/rio/...

# Run with coverage
make cover
```

---

## License

© 2025 **Checker Corp** — Proprietary and Confidential.

All rights reserved. Redistribution prohibited without written consent.

See [LICENSE](LICENSE) and [NOTICE](NOTICE) files for details.
