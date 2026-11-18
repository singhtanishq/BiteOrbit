# ADR 004 — Server-Sent Events for real-time updates (WebSocket declined)

Status: Accepted · Date: 2026-11-19

## Context

Customers, restaurants and riders need live order/delivery updates. Options: WebSocket
(`gorilla/websocket` or `nhooyr/websocket`) vs SSE (`text/event-stream`).

## Decision

Use **SSE over the existing same-origin HTTP path** (`/api/v1/.../events`). SSE gives us
auto-reconnect semantics, works through the Next.js dev proxy, needs no upgrade handshake,
and all payloads are server-authoritative snapshots with sequence numbers. Polling remains
a universal fallback for every consumer.

## Consequences

- One less dependency; simpler auth (cookie headers flow naturally).
- Client→server messages still travel as normal POSTs (SSE is read-only by design).
- A single API process fans out via an in-process pub/sub (Redis pub/sub when configured).
