# Braza Adapter (placeholder)

This directory is reserved for the **Braza adapter**, which will be moved into this monorepo from its standalone repository.

Before the move, tag the current stable version in the braza-adapter repo:

- If the repo has version tags (e.g. `v2.0.0`): tag with `<version>-pre-commons` (e.g. `v2.0.0-pre-commons`).
- If it has no version tags: tag with `pre-commons`.

After copying the code here, update the Go module path to `github.com/Checker-Finance/adapters/braza-adapter` and adjust ArgoCD/CI to use this repo and path `braza-adapter/`.
