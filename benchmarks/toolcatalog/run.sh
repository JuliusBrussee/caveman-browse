#!/usr/bin/env bash
# Tier 1 head-to-head: MCP tool-catalog token cost.
# Fetches each server's live tools/list and counts the identical serialization
# (compact JSON [{name,description,inputSchema}]) with the offline o200k_base
# counter. Every figure is inferred. Versions are whatever npx resolves; the
# RESULTS.md table records the exact versions measured.
set -euo pipefail
cd "$(dirname "$0")/../.."

go build -o bin/caveman-browse ./cmd/caveman-browse

fetch() { node benchmarks/toolcatalog/fetch-tools.mjs "$@"; }
count() { go run ./benchmarks/toolcatalog/count; }

echo "== caveman-browse (this repo, HEAD)"
CAVEMAN_HOME="$(mktemp -d)" fetch ./bin/caveman-browse | count

echo "== @playwright/mcp"
fetch npx -y @playwright/mcp@latest | count

echo "== chrome-devtools-mcp"
fetch npx -y chrome-devtools-mcp@latest | count
