#!/usr/bin/env bash
# Verifies the committed OpenAPI document covers every route the API serves.
# Fails (non-zero) when a route is missing from docs/api/openapi.yaml.
set -euo pipefail
cd "$(dirname "$0")/../services/api"

export BITEORBIT_DATABASE_URL="${BITEORBIT_DATABASE_URL:-postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit?sslmode=disable}"
go test ./internal/openapi -run TestRoutesCoveredByOpenAPI -count=1
