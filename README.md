# Checker Adapters

Monorepo for Checker trading platform adapters. Each adapter integrates with an external venue (Rio, Braza, etc.) and exposes a consistent interface to the rest of the stack.

## Adapters

| Adapter | Description |
|---------|-------------|
| [rio-adapter](./rio-adapter/) | Integrates with Rio Bank's FXCore API. Quote creation, order execution, webhooks, NATS events. |
| [braza-adapter](./braza-adapter/) | Braza venue adapter. Stub in place; replace with full implementation from standalone repo when ready. |

## Repository layout

```
adapters/
├── rio-adapter/       # Rio adapter service
├── braza-adapter/     # Braza adapter (stub + k8s/CI/ArgoCD)
├── scripts/          # OIDC and repo setup
└── README.md
```

## Development

Each adapter is self-contained with its own `go.mod`, Dockerfile, and k8s manifests. Work from the adapter directory:

```bash
cd rio-adapter
make build
make test
```

See each adapter's README for configuration and run instructions.

## CI / Deployment

- **rio-adapter:** Build and push from `rio-adapter/`; ArgoCD apps point at this repo with paths `rio-adapter/k8s/overlays/dev` and `rio-adapter/k8s/overlays/prod`.
- **braza-adapter:** Workflow `build-and-push-braza-adapter.yml`; ArgoCD apps at `braza-adapter/k8s/overlays/dev` and `braza-adapter/k8s/overlays/prod`. Set GitHub secret `ECR_REPOSITORY_BRAZA` (e.g. `braza-adapter`) and create ECR repo; create AWS Secrets Manager entries `braza-adapter/dev` and `braza-adapter/prod` with keys used by ExternalSecrets (see braza-adapter README).

## History

- **rio-adapter** was moved from [Checker-Finance/rio-adapter](https://github.com/Checker-Finance/rio-adapter). Last standalone release: tag `v1.0.0-pre-commons`.
- **braza-adapter** will be moved from its standalone repo; tag with `<version>-pre-commons` or `pre-commons` before the move.
