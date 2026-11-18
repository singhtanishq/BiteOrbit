#!/usr/bin/env bash
# Seed the local database with the synthetic BiteOrbit demo dataset.
# Refuses to run unless the API is configured for a local/dev environment.
set -euo pipefail
cd "$(dirname "$0")/../services/api"

export BITEORBIT_DATABASE_URL="${BITEORBIT_DATABASE_URL:-postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit?sslmode=disable}"
export BITEORBIT_ENV="${BITEORBIT_ENV:-development}"

exec go run ./cmd/seed "$@"
