# BiteOrbit — local operations runbook

## Prerequisites

- Docker Desktop (or any Docker engine) running
- Node.js ≥ 20 + pnpm ≥ 9 (corepack enable)
- Go ≥ 1.27

## One-time setup

```bash
pnpm install                 # workspace dependencies
docker compose up -d         # PostgreSQL (54329), Redis (54380), Mailpit (51080 UI / 51025 SMTP)
bash scripts/migrate.sh up   # apply schema migrations
bash scripts/seed.sh         # load the synthetic BiteOrbit demo dataset
```

## Daily development

```bash
pnpm dev        # API :8080 + customer :3000 + restaurant :3001 + courier :3002 + admin :3003
```

## Reset everything

```bash
pnpm reset      # volumes down -v, up, migrate, seed — deterministic data
```

## Component status checks

| Component | How to check |
| --- | --- |
| API liveness | `curl http://127.0.0.1:8080/api/v1/healthz` |
| API + DB readiness | `curl http://127.0.0.1:8080/api/v1/readyz` |
| Postgres | `docker compose exec postgres pg_isready -U biteorbit` |
| Redis | `docker compose exec redis redis-cli ping` |
| Mailpit | open http://127.0.0.1:51080 (captured demo email) |

## Logs

- API logs are structured JSON on stdout (`pnpm dev` prefixes them `[api]`).
- Per-request correlation: every response carries `request_id` (also `X-Request-ID`).

## Common issues

| Symptom | Fix |
| --- | --- |
| `database unreachable` on API boot | Wait for `pg_isready`; scripts/api-dev.sh retries ~30s |
| Migration failure `42703` column missing | Stale binary — migrations are embedded; rebuild `go build ./cmd/...` |
| Seed fails mid-way | Nothing is committed (single transaction); fix and rerun `pnpm seed` |
| Port conflict on 54329/54380/51025 | Change mapping in docker-compose.yml and BITEORBIT_* URLs |
| Cookies not persisting | Apps proxy `/api/v1/*` same-origin; check `API_ORIGIN` env for the rewrites |

## What is simulated locally (never real)

- Payments: `mock` adapter — signed demo webhooks, no money movement
- SMS/OTP: `fake` adapter — logged only, never transmitted
- Email: Mailpit capture server — messages never leave the machine
- Maps/GPS: deterministic demo simulator — no live traffic or turn-by-turn

## Backup / restore (local)

```bash
# backup
docker compose exec -T postgres pg_dump -U biteorbit biteorbit > backup.sql
# restore
cat backup.sql | docker compose exec -T postgres psql -U biteorbit -d biteorbit
```

Production backup strategy (managed snapshots + object storage versioning) is documented
in the deployment notes (docs/operations/production-checklist.md).
