# ADR 000 — Starting state, environment and working approach

Date: 2026-11-19
Status: Accepted

## Verified facts

These were inspected directly before any project change.

- **Repository**: `/Users/tanishqsingh/Desktop/Personal/Projects/BiteOrbit` is an existing Git
  repository on branch `main` with a clean working tree.
- **History**: three existing commits (`8e23cbc`, `8b6fcc9`, `03f2cf7`, all "Initial commit").
  History must be preserved; no reinitialisation, force-push or history rewrite.
- **Remote**: `origin` = `git@github-personal:singhtanishq/BiteOrbit.git`, verified with
  `git ls-remote origin HEAD` → `03f2cf7…` which equals the local `main` HEAD, so the remote is
  the intended BiteOrbit repository and is in sync with local history.
- **Authentication**: SSH to GitHub via the `github-personal` host alias
  (`~/.ssh/config`, identity `~/.ssh/id_ed25519_github_personal`) authenticates as
  `singhtanishq`. Pushing to `origin/main` is therefore possible and authorised by the operator.
- **Tracked files**: only `.gitignore`. `BiteOrbit.md` (the private master directive) is present
  on disk, **untracked**, and is already ignored by the first line of `.gitignore`
  (verified with `git check-ignore -v BiteOrbit.md`).
- **Host**: macOS 27.2.1 (Darwin 27.2.0), Apple Silicon (`arm64`).

## Toolchain inventory

| Tool | Version | Status |
| --- | --- | --- |
| Node.js | v24.22.1 | available |
| npm | 11.19.1 | available |
| pnpm | 12.18.0 | available (workspaces supported) |
| Go | 1.27.2 darwin/arm64 | available |
| Docker | 29.9.3 | installed; daemon started via Docker Desktop at M00 |
| Docker Compose | v5.7.0 | available |
| Git | 2.54.0 (Apple Git-157) | available |
| Python | 3.14.7 | available (not required by the stack) |
| Mailpit | installed via Homebrew | available as a non-Docker fallback for local email capture |
| Yarn | not installed | not needed |
| PostgreSQL / Redis (native) | not installed | provided through Docker Compose |

## Decisions

1. **Stack**: adopt the directive's preferred stack — pnpm workspaces monorepo, Go backend
   (versioned REST API), PostgreSQL (migrations + seed), Redis (rate limiting / ephemeral
   coordination with an in-process fallback so Docker-less runs still work), four Next.js
   (App Router, TypeScript, Tailwind) web apps for customer / restaurant / courier / admin.
2. **Turborepo is omitted.** Plain pnpm workspaces plus small package scripts cover the build
   needs at this scale; Turborepo would add a dependency without measurable value yet (ADR 002).
3. **Local object storage** uses a filesystem adapter (`data/uploads/`, git-ignored) behind a
   provider interface; MinIO remains the documented production path (ADR 003).
4. **Real-time updates** use Server-Sent Events with polling fallback (simpler, firewall-friendly,
   no extra dependency) — ADR 004.
5. **Docker Compose** provides PostgreSQL + Redis + Mailpit for local development; the Go API
   runs natively on the host for fast iteration.
6. **The private directive is never committed.** `BiteOrbit.md` stays ignored; `.gitignore` was
   merged with additional safe patterns (deps, build output, env files, uploads, logs, test
   artefacts) without removing the existing rule.
7. **Commits** are small and verified per milestone and pushed to `origin/main` (verified above).

## Assumptions

- The demo city, restaurants, people, coupons and reviews are entirely synthetic and clearly
  labelled as demo data in-product and in documentation.
- INR with integer paise is the canonical money representation; demo timezone `Asia/Kolkata`.
- No real payment/SMS/maps/email credentials exist; all such integrations are local adapters
  behind provider interfaces, clearly labelled as simulated.
