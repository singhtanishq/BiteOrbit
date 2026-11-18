#!/usr/bin/env bash
# Full local reset: wipe volumes, migrate from scratch, reseed demo data.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> Stopping and removing volumes"
docker compose down -v

echo "==> Starting Postgres, Redis, Mailpit"
docker compose up -d

echo "==> Waiting for Postgres readiness"
for _ in $(seq 1 60); do
  if docker compose exec -T postgres pg_isready -U biteorbit -d biteorbit >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> Migrating"
bash scripts/migrate.sh up

echo "==> Seeding demo data"
bash scripts/seed.sh

echo "==> Done. Start the apps with: pnpm dev"
