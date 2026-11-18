# ADR 007 — Four separate portal apps instead of one mega-app

Status: Accepted · Date: 2026-11-19

## Context

The directive's preferred stack lists four web apps. One Next.js app with route groups
would share code more easily but mixes four very different audiences, bundles and
deployment needs.

## Decision

Build four separate apps (`customer-web`, `restaurant-web`, `courier-web`, `admin-web`)
sharing `@biteorbit/ui` and `@biteorbit/api-client`. Each app ships only what its
audience needs.

## Consequences

- Smaller bundles per audience; independent deployability.
- A little duplication (layout shells) accepted in exchange for clarity.
- Route-level authorisation in each app is UX only; real authorisation is enforced by the
  API for every request regardless of which app called it.
