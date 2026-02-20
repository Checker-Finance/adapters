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

## 2. GitHub Actions secrets (OIDC – no long-lived keys)

The workflow uses **OIDC** to assume an IAM role. You only need:

| Secret           | Description |
|------------------|-------------|
| `ECR_REPOSITORY` | ECR repo name (e.g. `rio-adapter`) – already set |
| `AWS_ACCOUNT_ID` | AWS account ID – already set |
| `AWS_REGION`     | ECR region (e.g. `us-east-2`) – already set |
| `AWS_ROLE_ARN`   | ARN of the IAM role created by `scripts/setup-oidc-aws.sh` |

**One-time AWS setup (CLI):** From the adapters repo, run (with AWS CLI already logged in, e.g. `aws sso login`):

```bash
./scripts/setup-oidc-aws.sh
```

Then set the printed role ARN as the `AWS_ROLE_ARN` secret:

```bash
gh secret set AWS_ROLE_ARN --repo Checker-Finance/adapters --body "arn:aws:iam::730335471935:role/GitHubActions-adapters-ECR"
```

(Use the exact ARN printed by the script.) You can remove `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from the repo secrets if they were set before.

## 3. ArgoCD

Point the existing rio-adapter applications to this repo:

- **Repo URL:** `https://github.com/Checker-Finance/adapters.git`
- **Path (dev):** `rio-adapter/k8s/overlays/dev`
- **Path (prod):** `rio-adapter/k8s/overlays/prod`

The manifests in this repo (`rio-adapter/k8s/argocd/application-*.yaml`) already use these values. If your ArgoCD apps are defined elsewhere, update them to match.

## 4. Braza-adapter

Scaffolding is in place: stub app, CI workflow, Kustomize base/overlays, and ArgoCD apps.

**To enable CI:**

1. Create an ECR repository (e.g. `braza-adapter`) in the same account/region.
2. Set GitHub secret: `ECR_REPOSITORY_BRAZA` = ECR repo name (e.g. `braza-adapter`).

**To deploy:** Create AWS Secrets Manager entries `braza-adapter/dev` and `braza-adapter/prod` with the keys referenced in `braza-adapter/k8s/overlays/*/externalsecret.yaml` (e.g. `DATABASE_URL`, `BRAZA_API_KEY`, `REDIS_PASS`). Apply the ArgoCD applications under `braza-adapter/k8s/argocd/`.

**To replace the stub with the full adapter:**

1. In the **standalone braza-adapter** repo: tag current stable with `<version>-pre-commons` or `pre-commons`.
2. Copy its code into `adapters/braza-adapter/` (keep module path `github.com/Checker-Finance/adapters/braza-adapter`).
3. Update imports to `github.com/Checker-Finance/adapters/braza-adapter`.
4. Adjust Dockerfile/Makefile and k8s ExternalSecrets if the real service needs different env or secrets.
