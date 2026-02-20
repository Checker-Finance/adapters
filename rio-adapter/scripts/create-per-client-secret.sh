#!/usr/bin/env bash
# Create a per-client AWS Secrets Manager secret for Rio adapter.
# Uses API key (and optional base_url) from the existing rio-adapter/prod secret.
#
# Usage:
#   ./scripts/create-per-client-secret.sh [CLIENT_ID]
# Default CLIENT_ID: 94d81a58-1935-4b79-9a26-0f2d27414218
#
# Requires: aws CLI, jq
# Region: us-east-2 (override with AWS_REGION)

set -euo pipefail

CLIENT_ID="${1:-94d81a58-1935-4b79-9a26-0f2d27414218}"
ENV="${ENV:-prod}"
REGION="${AWS_REGION:-us-east-2}"
SOURCE_SECRET="rio-adapter/prod"
# Prod Rio API base URL (from k8s overlay); use /api path for API calls
DEFAULT_BASE_URL="https://app.rio.trade/api"

echo "Fetching API key from existing secret: $SOURCE_SECRET"
CURRENT=$(aws secretsmanager get-secret-value --secret-id "$SOURCE_SECRET" --region "$REGION" --query SecretString --output text)

API_KEY=$(echo "$CURRENT" | jq -r '.RIO_API_KEY // empty')
if [[ -z "$API_KEY" ]]; then
  echo "ERROR: $SOURCE_SECRET has no RIO_API_KEY. Keys present: $(echo "$CURRENT" | jq -r 'keys | join(", ")')"
  exit 1
fi

# Use base_url from current secret if present, else default
BASE_URL=$(echo "$CURRENT" | jq -r '.RIO_BASE_URL // .base_url // empty')
[[ -z "$BASE_URL" ]] && BASE_URL="$DEFAULT_BASE_URL"

# Ensure base_url has /api path if it looks like app host only
if [[ "$BASE_URL" == https://app.rio.trade ]]; then
  BASE_URL="https://app.rio.trade/api"
fi

SECRET_NAME=$(echo "${ENV}/${CLIENT_ID}/rio" | tr '[:upper:]' '[:lower:]')
SECRET_JSON=$(jq -n \
  --arg api_key "$API_KEY" \
  --arg base_url "$BASE_URL" \
  --arg country "US" \
  '{api_key: $api_key, base_url: $base_url, country: $country}')

echo "Creating per-client secret: $SECRET_NAME"
echo "  base_url: $BASE_URL"
echo "  country:  US"

if aws secretsmanager describe-secret --secret-id "$SECRET_NAME" --region "$REGION" 2>/dev/null; then
  echo "Secret already exists. Updating value..."
  aws secretsmanager put-secret-value \
    --secret-id "$SECRET_NAME" \
    --region "$REGION" \
    --secret-string "$SECRET_JSON"
  echo "Done (updated)."
else
  aws secretsmanager create-secret \
    --name "$SECRET_NAME" \
    --region "$REGION" \
    --secret-string "$SECRET_JSON"
  echo "Done (created)."
fi
