# Checker Adapters

Monorepo for Checker trading platform adapters. Each adapter integrates with an external venue (Rio, Braza, etc.) and exposes a consistent interface to the rest of the stack.

## Adapters

| Adapter | Description |
|---------|-------------|
| [rio-adapter](./rio-adapter/) | Integrates with Rio Bank's FXCore API. Quote creation, order execution, webhooks, NATS events. |
| [braza-adapter](./braza-adapter/) | *(To be added)* Braza venue adapter. |

## Repository layout

```
adapters/
├── rio-adapter/       # Rio adapter service
├── braza-adapter/     # Braza adapter (placeholder)
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
- **braza-adapter:** To be wired when the adapter is added.

## History

- **rio-adapter** was moved from [Checker-Finance/rio-adapter](https://github.com/Checker-Finance/rio-adapter). Last standalone release: tag `v1.0.0-pre-commons`.
- **braza-adapter** will be moved from its standalone repo; tag with `<version>-pre-commons` or `pre-commons` before the move.
