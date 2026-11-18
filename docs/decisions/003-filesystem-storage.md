# ADR 003 — Filesystem object storage locally, provider interface for production

Status: Accepted · Date: 2026-11-19

## Context

Restaurant/dish images and (later) proof-of-delivery media need storage. MinIO in Docker
would work, but adds an extra container and credential surface for the local demo.

## Decision

Implement a small `MediaStore` provider interface (`Put`, `Get`, `Delete`, signed-style
private reads) with a **filesystem adapter** writing under `data/uploads/` (git-ignored).
Production documents MinIO/S3 as the swap-in adapter.

## Consequences

- Local demo runs with zero extra infrastructure.
- Upload validation (type allowlist, size limits, generated keys) lives in the API layer,
  identical for any adapter.
