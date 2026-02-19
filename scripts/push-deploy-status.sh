#!/usr/bin/env bash
# Fetches site status from local EzWeb and dispatches it to the profile repo.
# Intended to run via cron every 30 minutes.
#
# Requires: GITHUB_TOKEN env var (PAT with contents:write on JadenRazo/JadenRazo)

set -euo pipefail

EZWEB_URL="http://localhost:8088/api/status"
REPO="JadenRazo/JadenRazo"

if [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "ERROR: GITHUB_TOKEN not set" >&2
  exit 1
fi

STATUS=$(curl -sf --max-time 10 "$EZWEB_URL" || echo "[]")

# Dispatch to profile repo with status data in the payload
curl -sf -X POST \
  -H "Accept: application/vnd.github.v3+json" \
  -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/${REPO}/dispatches" \
  -d "$(jq -n --argjson status "$STATUS" '{event_type: "deploy-status", client_payload: {status: $status}}')"

echo "Dispatched deploy status with $(echo "$STATUS" | jq length) site(s)"
