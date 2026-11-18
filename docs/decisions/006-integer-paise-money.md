# ADR 006 — Money as integer paise, server-authoritative pricing

Status: Accepted · Date: 2026-11-19

## Context

Floating-point money causes rounding drift. The directive mandates paise as the canonical
unit and server authority over every amount.

## Decision

- All monetary columns are `BIGINT` paise with an explicit currency code (`INR`).
- Go computes every quote, discount, tax, fee and refund with integer math and a documented
  rounding policy (round half up at each tax/fee line; discounts applied before taxes).
- The frontend's `formatINR` (in `@biteorbit/ui`) is presentation-only; the server
  recomputes and validates everything at quote and placement time, and orders store a
  full price snapshot so later rule changes never alter history.

## Consequences

- No decimal library needed in Go or TS.
- Financial aggregates are testable with exact expected integers.
