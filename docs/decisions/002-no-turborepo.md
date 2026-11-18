# ADR 002 — No Turborepo (yet)

Status: Accepted · Date: 2026-11-19

## Context

The directive permits Turborepo "if it adds no value". With four small Next.js apps and
one Go module, full builds take under two minutes and caching gains are marginal.

## Decision

Use plain pnpm workspaces and package scripts. Revisit Turborepo if build times or
task graphs become a bottleneck.

## Consequences

- One less dependency and config surface.
- CI runs tasks sequentially; acceptable at current scale.
