# Adapter Reference

Complete reference for all adapters in this monorepo: HTTP endpoints, NATS subjects, ports, and authentication.

## Table of Contents

- [Rio](#rio)
- [Braza](#braza)
- [XFX](#xfx)
- [Zodia](#zodia)
- [Kiiex](#kiiex)
- [B2C2](#b2c2)
- [At a Glance](#at-a-glance)

---

## Rio

**Port:** `9010` (`RIO_PORT`)
**Auth:** API key per client — resolved from AWS Secrets Manager at `{env}/{clientId}/rio`
**Status tracking:** Webhooks (primary) + polling fallback (`RIO_POLL_INTERVAL`, default 30s)

### HTTP Endpoints

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

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound | `cmd.lp.quote_request.v1.RIO` |
| Outbound (interim) | `evt.trade.status_changed.v1.RIO` |
| Outbound (final) | `evt.trade.filled.v1.RIO` |
| Outbound (final) | `evt.trade.rejected.v1.RIO` |
| Outbound (final) | `evt.trade.cancelled.v1.RIO` |
| Outbound (final) | `evt.trade.refunded.v1.RIO` |

---

## Braza

**Port:** `9010` (`BRAZA_PORT`)
**Auth:** API key per client — resolved from AWS Secrets Manager at `{env}/{clientId}/braza`
**Status tracking:** Polling only (`POLL_INTERVAL`, default 5m)

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS + store status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List available products |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ |
| `POST` | `/api/v1/orders` | Execute order |
| `POST` | `/api/v1/resolve-order/:quoteId` | Resolve/finalize order |

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound | `cmd.lp.quote_request.v1.BRAZA` |
| Outbound (interim) | `evt.trade.status_changed.v1.BRAZA` |
| Outbound (final) | `evt.trade.filled.v1.BRAZA` |
| Outbound (final) | `evt.trade.rejected.v1.BRAZA` |
| Outbound (final) | `evt.trade.cancelled.v1.BRAZA` |
| Outbound (final) | `evt.trade.refunded.v1.BRAZA` |

---

## XFX

**Port:** `9030` (`XFX_PORT`)
**Auth:** OAuth2 Client Credentials via Auth0 — tokens cached per client (24h, 5-min refresh buffer)
**Status tracking:** Polling only — no webhooks (`XFX_POLL_INTERVAL`, default 15s)
**Supported pairs:** USD/MXN, USDT/MXN, USDC/MXN, USD/COP, USDT/COP, USDC/COP, USD/USDT, USD/USDC (min $100,000 USD)
**Trading hours:** 13:00–01:00 UTC

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS + store status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List supported pairs (static, hardcoded) |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ (15s validity window) |
| `POST` | `/api/v1/orders` | Execute order |
| `POST` | `/api/v1/resolve-order/:quoteId` | Resolve/finalize order |

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound (quote request) | `cmd.lp.quote_request.v1.XFX` |
| Inbound (trade execute) | `cmd.lp.trade_execute.v1.XFX` |
| Outbound (interim) | `evt.trade.status_changed.v1.XFX` |
| Outbound (final) | `evt.trade.filled.v1.XFX` |
| Outbound (final) | `evt.trade.rejected.v1.XFX` |
| Outbound (final) | `evt.trade.cancelled.v1.XFX` |

---

## Zodia

**Port:** `9040` (`ZODIA_PORT`)
**Auth:** HMAC-SHA512 for REST (`Rest-Key`/`Rest-Sign` headers); WebSocket token via `POST /ws/auth`
**Status tracking:** Webhooks (primary) + polling fallback (`ZODIA_POLL_INTERVAL`, default 15s)
**Pair format:** Zodia uses dots (`USD.MXN`); canonical uses colons (`USD:MXN`)

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS + store status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List available instruments |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ (via WebSocket RFS) |
| `POST` | `/api/v1/orders` | Execute order (via WebSocket) |
| `POST` | `/api/v1/resolve-order/:quoteId` | Resolve/finalize order |
| `POST` | `/webhooks/zodia/transactions` | Zodia webhook (Redis dedup, 48h TTL) |

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound (quote request) | `cmd.lp.quote_request.v1.ZODIA` |
| Inbound (trade execute) | `cmd.lp.trade_execute.v1.ZODIA` |
| Outbound | `evt.trade.*.v1.ZODIA` |

---

## Kiiex

**Port:** `8082` (`SERVER_PORT`)
**Auth:** HMAC (AlphaPoint/Kiiex WebSocket session) — per-client secrets from AWS Secrets Manager at `{env}/{clientId}/kiiex`
**Transport:** AlphaPoint WebSocket — no REST polling, no webhooks
**Note:** Minimal HTTP surface; quote creation happens entirely via NATS → WebSocket

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS status |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/api/v1/orders` | Execute order |

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound (execute) | `cmd.lp.trade_execute.v1.KIIEX` |
| Inbound (cancel) | `cmd.lp.trade_cancel.v1.KIIEX` |
| Outbound | `evt.trade.filled.v1.KIIEX` |
| Outbound | `evt.trade.cancelled.v1.KIIEX` |

---

## B2C2

**Port:** `9050` (`HEALTH_PORT`)
**Auth:** Static API token per client (`Authorization: Token <api_token>`) — from AWS Secrets Manager at `{env}/{clientId}/b2c2`
**Order model:** Fill-or-Kill (FOK) — synchronous, no polling needed
**Instrument format:** Canonical `usd:btc` → B2C2 `USDBTC.SPOT`

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check — reports NATS status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/v1/products` | List instruments (fetched from B2C2 API) |
| `GET` | `/api/v1/balances/:client_id` | Client balances |
| `POST` | `/api/v1/quotes` | Create RFQ |
| `POST` | `/api/v1/orders` | Execute order (FOK, synchronous — `executed_price != null` → filled, `null` → cancelled) |

### NATS

| Direction | Subject |
|-----------|---------|
| Inbound (quote request) | `cmd.lp.quote_request.v1.B2C2` |
| Inbound (trade execute) | `cmd.lp.trade_execute.v1.B2C2` |
| Inbound (cancel) | `cmd.lp.trade_cancel.v1.B2C2` |
| Outbound | `evt.trade.quote_ready.v1.B2C2` |
| Outbound | `evt.trade.filled.v1.B2C2` |
| Outbound | `evt.trade.cancelled.v1.B2C2` |

---

## At a Glance

| Adapter | Port | Webhooks | Products source | Status tracking | Auth model |
|---------|------|----------|-----------------|-----------------|------------|
| Rio | 9010 | Yes | Dynamic | Webhook + poll | API key |
| Braza | 9010* | No | Dynamic | Poll only | API key |
| XFX | 9030 | No | Static (hardcoded) | Poll only | OAuth2 / Auth0 |
| Zodia | 9040 | Yes | Dynamic | Webhook + poll | HMAC |
| Kiiex | 8082 | No | — | WS events | HMAC (AlphaPoint) |
| B2C2 | 9050 | No | Dynamic (B2C2 API) | Sync (FOK) | Static token |

\* Braza shares the same default port as Rio but is configured separately via `BRAZA_PORT`.
