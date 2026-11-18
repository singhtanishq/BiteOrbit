#!/usr/bin/env bash
# Apply (or check) database migrations against the local Postgres.
# Usage: scripts/migrate.sh up | status
set -euo pipefail
cd "$(dirname "$0")/../services/api"

export BITEORBIT_DATABASE_URL="${BITEORBIT_DATABASE_URL:-postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit?sslmode=disable}"

case "${1:-up}" in
  up) exec go run ./cmd/migrate up ;;
  status) exec go run ./cmd/migrate status ;;
  *) echo "usage: migrate.sh up|status" >&2; exit 2 ;;
esac
