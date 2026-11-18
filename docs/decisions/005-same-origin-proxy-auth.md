# ADR 005 — Same-origin API proxy and HttpOnly cookie sessions

Status: Accepted · Date: 2026-11-19

## Context

Sessions must be HttpOnly cookies (no token storage in JS). Cookies between
`localhost:3000` and `localhost:8080` would be cross-site and need `SameSite=None; Secure`,
which requires TLS even in dev.

## Decision

Each Next.js app rewrites `/api/v1/*` to the Go API (`API_ORIGIN`). The browser only ever
speaks to its own origin, so `SameSite=Lax` HttpOnly cookies work in dev and production
(reverse proxy does the same job when deployed). The Go API additionally validates
`Origin`/`Referer` on state-changing requests against an allowlist (CSRF defence-in-depth)
and sets `SameSite=Lax; HttpOnly; Secure` (Secure when the request is HTTPS).

## Consequences

- No CORS preflights, no CSRF token plumbing, works identically for all four apps.
- SSE and file downloads share the same cookie path.
- Production topology: put each app behind its own hostname/subdomain with the API behind
  the same hostname's `/api/v1/*` path (documented in docs/operations).
