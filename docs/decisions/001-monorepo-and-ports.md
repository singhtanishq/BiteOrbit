# ADR 001 — pnpm workspace monorepo, one Go module, fixed ports

Status: Accepted · Date: 2026-11-19

## Context

BiteOrbit needs four coordinated web experiences plus a Go API, a worker and shared
frontend code. The team is small (one engineer + automation); build tooling should not
outweigh the product.

## Decision

- One pnpm workspace with `apps/*` (four Next.js apps), `packages/*` (`ui`, `api-client`)
  and `e2e` (Playwright).
- One Go module at `services/api` containing `cmd/api`, `cmd/migrate`, `cmd/seed`,
  `cmd/worker` and all domain packages.
- Fixed dev ports: customer 3000, restaurant 3001, courier 3002, admin 3003, API 8080;
  Postgres on host port 54329, Redis 54380, Mailpit SMTP 51025 / UI 51080.

## Consequences

- Simple commands (`pnpm dev`, `pnpm test`) work everywhere.
- Cross-package TypeScript imports are workspace deps; the Go module is self-contained.
- If builds grow, Turborepo can be layered on without restructuring (see ADR 002).
