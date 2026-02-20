# Adapters repo – setup and next steps

## 1. Create the GitHub repo and push

The **adapters** repo does not exist on GitHub yet. Create it, then push:

1. On GitHub: **New repository** → name: **adapters**, org: **Checker-Finance**. Do not add a README (you already have one locally).
2. Push from your machine:

```bash
cd /Users/gkoutepov/checker/adapters
git remote add origin https://github.com/Checker-Finance/adapters.git   # or set-url if already added
git push -u origin main
```

## 2. GitHub Actions secrets

In **GitHub → adapters → Settings → Secrets and variables → Actions**, add (same values as in the old rio-adapter repo if you want the same ECR image):

| Secret               | Description                    |
|----------------------|--------------------------------|
| `ECR_REPOSITORY`     | ECR repo name (e.g. rio-adapter) |
| `AWS_ACCOUNT_ID`     | AWS account ID                |
| `AWS_REGION`         | ECR region (e.g. us-east-2)   |
| `AWS_ACCESS_KEY_ID`  | IAM key for ECR push           |
| `AWS_SECRET_ACCESS_KEY` | IAM secret for ECR push    |

## 3. ArgoCD

Point the existing rio-adapter applications to this repo:

- **Repo URL:** `https://github.com/Checker-Finance/adapters.git`
- **Path (dev):** `rio-adapter/k8s/overlays/dev`
- **Path (prod):** `rio-adapter/k8s/overlays/prod`

The manifests in this repo (`rio-adapter/k8s/argocd/application-*.yaml`) already use these values. If your ArgoCD apps are defined elsewhere, update them to match.

## 4. Braza-adapter (later)

When you add braza-adapter:

1. In the **braza-adapter** repo: tag current stable with `<version>-pre-commons` or `pre-commons`.
2. Copy its code into `adapters/braza-adapter/`.
3. Update `go.mod` and imports to `github.com/Checker-Finance/adapters/braza-adapter`.
4. Add a CI workflow (e.g. `build-and-push-braza-adapter.yml`) and ArgoCD apps as needed.
