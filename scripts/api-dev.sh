#!/usr/bin/env bash
# Runs the Go API with a short startup delay so Postgres is ready in fresh setups.
set -euo pipefail
cd "$(dirname "$0")/../services/api"

# Wait briefly for Postgres (best effort; the API also retries on startup).
for _ in $(seq 1 30); do
  if nc -z 127.0.0.1 54329 2>/dev/null; then break; fi
  sleep 1
done

export BITEORBIT_DATABASE_URL="${BITEORBIT_DATABASE_URL:-postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit?sslmode=disable}"
export BITEORBIT_REDIS_URL="${BITEORBIT_REDIS_URL:-redis://127.0.0.1:54380/0}"
export BITEORBIT_HTTP_ADDR="${BITEORBIT_HTTP_ADDR:-127.0.0.1:8080}"
export BITEORBIT_ENV="${BITEORBIT_ENV:-development}"

exec go run ./cmd/api
