#!/usr/bin/env bash
# Verifies docs/api/openapi.json matches every route the API serves.
# Regenerate with: REGENERATE=1 go test ./internal/server -run TestOpenAPIMatchesCommittedDoc
set -euo pipefail
cd "$(dirname "$0")/../services/api"
go test ./internal/server -run TestOpenAPIMatchesCommittedDoc -count=1
