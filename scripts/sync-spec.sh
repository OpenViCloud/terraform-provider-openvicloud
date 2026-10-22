#!/bin/bash
# Copies the public API contract api/openapi-v1.json from one commit of OpenViCloud/titan-control-plane, records that
# commit in api/SOURCE, and regenerates the client. CI never fetches the contract: it only checks that the committed
# client matches the committed spec.
# Usage: scripts/sync-spec.sh <control-plane commit, tag or branch>
set -euo pipefail
ref=${1:?usage: scripts/sync-spec.sh <control-plane commit, tag or branch>}
cd "$(dirname "$0")/.."
sha=$(gh api "repos/OpenViCloud/titan-control-plane/commits/$ref" --jq .sha)
gh api "repos/OpenViCloud/titan-control-plane/contents/api/openapi-v1.json?ref=$sha" \
  -H "Accept: application/vnd.github.raw" > api/openapi-v1.json
echo "$sha" > api/SOURCE
go generate ./api/
echo "api/openapi-v1.json synced from OpenViCloud/titan-control-plane@$sha; mention it in the pull request"
