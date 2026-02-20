# Copilot Instructions for Rio Adapter

This doc orients AI assistants working on the Rio Adapter (Checker -> Rio Trading API bridge). Keep it concise and code-oriented.

## Architecture & Data Flow
- Entry point: cmd/rio-adapter/main.go wires config, logger, NATS, rate limiter, hybrid store (Redis + Postgres), service, poller (webhook fallback), webhook handler, and Fiber HTTP server.
- Service layer: internal/rio/service.go orchestrates quote creation and order execution via internal/rio/client.go, maps responses with internal/rio/mapper.go, and publishes NATS events through internal/publisher.
- Status handling: Webhooks (preferred) hit internal/rio/webhook_handler.go, which cancels pollers and publishes status_changed and terminal events; poller (internal/rio/poller.go) runs as fallback and emits the same subjects.
- Legacy sync: terminal trades are upserted into legacy activity tables via internal/legacy/trade_sync_writer.go; avoid breaking these schemas.
- Store: internal/store/store.go provides Redis-first cache plus Postgres persistence for balances, products, quotes/orders.
- Metrics: internal/metrics exposes Prometheus metrics at /metrics (http server started in main). Key histograms/counters defined there.

## API Surface
- Health: GET /health returns ok.
- Quotes: POST /api/v1/quotes (RFQ create), POST /api/v1/quotes/:quotation_id/execute (execute existing quote). Requests mapped to pkg/model.RFQRequest; responses use mapper helpers.
- Webhooks: POST /webhooks/rio/orders receives Rio order updates; cancels polling and publishes terminal events.
- NATS subjects: status_changed -> evt.trade.status_changed.v1.RIO; terminal -> evt.trade.<status>.v1.RIO (status already normalized/lowercase).

## Configuration & Secrets
- Config loader: pkg/config/config.go (env-driven, dotenv supported). Required: RIO_API_KEY, DATABASE_URL for Postgres, REDIS_ADDR for Redis, NATS_URL for JetStream. Defaults: port 9010, RIO_BASE_URL https://app.sandbox.rio.trade/api, RIO_POLL_INTERVAL 30s.
- Webhooks optional: set RIO_WEBHOOK_URL to auto-register on startup; otherwise polling is primary.
- Rate limits: internal/rate defaults to 10 rps, burst 20, 1s cooldown per key.
- Secrets cache: pkg/secrets/cache.go simple TTL cache; ensure cleanup goroutine started if used.

## Events & Mapping
- Canonical models: pkg/model/* (Envelope, RFQRequest, Quote, TradeConfirmation, Balance, Product).
- Status normalization lives in internal/rio/mapper.go (NormalizeRioStatus, IsTerminalStatus). Keep Rio’s 40+ statuses mapped before emitting events or syncing legacy.
- Publish helpers: internal/publisher/publisher.go (Publish, PublishEnvelope, PublishBalanceUpdated). JetStream used if available; headers include correlation_id etc.

## Persistence
- Redis used for balance cache and generic JSON blobs; keys often prefixed balance:<tenant>:<client>:<venue>:<instrument>.
- Postgres (pgxpool) used for ledger.balance_event/snapshot, reference.venue_products, activity tables for legacy sync. Migrations live in /migrations.

## Build, Run, Test
- Common make targets: make build, make run (local), make test (race, verbose), make bench, make cover, make fmt, make lint (golangci-lint 5m), make up/down (NATS + Redis), make docker-build.
- Main binary outputs to ./bin/rio-adapter; Docker tag reads VERSION file and pushes to ECR repo configured in Makefile.

## Patterns & Conventions
- Logging: zap; structured fields prefixed rio.* for Rio flows. Use logger.S() or logger.L() helpers; initialize via logger.Init.
- Dependency injection via constructors (NewService, NewPoller, NewWebhookHandler, NewHybrid, NewTradeSyncWriter, NewManager in rate).
- Prefer webhook-first status handling; polling only as fallback. Poller uses sync.Map to prevent duplicate polling.
- When adding Rio API calls, record metrics (metrics.IncRioRequest/ObserveDuration) and respect rate limiter.
- Status events should always carry normalized status and include client_id/order_id/quote_id; terminal events must set final=true.

## Gotchas
- Do not proceed without RIO_API_KEY; main enforces fatal exit.
- Redis or Postgres may be absent in some envs; store methods guard but legacy sync assumes Postgres available.
- Quote/Order ID mapping to legacy uses activity tables; check store.GetQuoteByQuoteID/GetOrderIDByRFQ before altering schemas.
- Mapper defaults CurrencyAmount to fiat if unset; pay attention when adding pairs or amount semantics.
- Webhook client_id falls back to userId when clientReferenceId missing; keep this alignment when changing webhook handling.
