#!/usr/bin/env bash
# Setup GitHub Actions OIDC in AWS: create OIDC provider (if missing), IAM role, and ECR permissions.
# Run with AWS CLI already authenticated (e.g. aws sso login). Account must allow creating OIDC provider and IAM roles.
#
# Usage: ./scripts/setup-oidc-aws.sh
# After running, add the printed role ARN as GitHub secret AWS_ROLE_ARN in the adapters repo.

set -euo pipefail

ACCOUNT_ID="${AWS_ACCOUNT_ID:-$(aws sts get-caller-identity --query Account --output text)}"
REGION="${AWS_REGION:-us-east-2}"
OIDC_PROVIDER="token.actions.githubusercontent.com"
ROLE_NAME="GitHubActions-adapters-ECR"
REPO_SUB="repo:Checker-Finance/adapters:ref:refs/heads/main"

echo "Account: $ACCOUNT_ID  Region: $REGION"
echo "OIDC provider: $OIDC_PROVIDER"
echo "Role name: $ROLE_NAME"
echo "Trusted sub (branch): $REPO_SUB"
echo ""

# 1. Create OIDC provider for GitHub Actions (idempotent; ignore if exists)
OIDC_ARN="arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
if aws iam get-open-id-connect-provider --open-id-connect-provider-arn "$OIDC_ARN" 2>/dev/null; then
  echo "OIDC provider already exists: $OIDC_ARN"
else
  echo "Creating OIDC provider..."
  aws iam create-open-id-connect-provider \
    --url "https://${OIDC_PROVIDER}" \
    --client-id-list "sts.amazonaws.com" \
    --thumbprint-list "6938fd4d98bab03faadb97b34396831e3780aea1"
  echo "Created: $OIDC_ARN"
fi
echo ""

# 2. Trust policy: allow GitHub Actions from Checker-Finance/adapters (main branch)
TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "$OIDC_ARN"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
          "token.actions.githubusercontent.com:sub": "$REPO_SUB"
        }
      }
    }
  ]
}
EOF
)

# 3. Create IAM role (idempotent; delete and recreate if you need to change trust)
if aws iam get-role --role-name "$ROLE_NAME" 2>/dev/null; then
  echo "Role $ROLE_NAME already exists. To change trust policy, delete it and re-run."
else
  echo "Creating IAM role: $ROLE_NAME"
  aws iam create-role \
    --role-name "$ROLE_NAME" \
    --description "Role for GitHub Actions (adapters repo) to push to ECR" \
    --assume-role-policy-document "$TRUST_POLICY"
fi
echo ""

# 4. Attach managed policy for ECR push (all repos in account)
echo "Attaching ECR policy to role..."
aws iam attach-role-policy \
  --role-name "$ROLE_NAME" \
  --policy-arn "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser"
echo "Attached: AmazonEC2ContainerRegistryPowerUser"
echo ""

ROLE_ARN="arn:aws:iam::${ACCOUNT_ID}:role/${ROLE_NAME}"
echo "---"
echo "Done. Add this as a GitHub Actions secret in the adapters repo:"
echo ""
echo "  gh secret set AWS_ROLE_ARN --repo Checker-Finance/adapters --body \"$ROLE_ARN\""
echo ""
echo "Or in the GitHub UI: Settings → Secrets and variables → Actions → New repository secret"
echo "  Name:  AWS_ROLE_ARN"
echo "  Value: $ROLE_ARN"
echo "---"
