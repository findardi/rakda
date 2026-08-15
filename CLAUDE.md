# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Wadi (product name: **Riksa**) — a Virtual Data Room. Two-package monorepo:

- `web/` — SvelteKit 5 frontend. **Uses Bun, not npm** (`web/README.md` npm instructions are scaffold boilerplate).
- `server/` — Go 1.26 backend (Chi, PostgreSQL/pgx, Minio). Module path: `github.com/findardi/Riksa-App/server`.
- `brainstorm-folder/` — discussion and decision notes for the active phase. See "Discussion notes" below.

`AGENTS.md` is the pre-existing agent instructions file; keep the two in sync when editing either.

## Skills — invoke proactively, not on request

Before starting any task, check the installed-skills list: if a skill's trigger
description matches the task, invoke it FIRST instead of doing the work with
manual/built-in tools. The user must never have to ask for a skill by name.
Current examples (non-exhaustive — apply to whatever is installed):

- **Codebase questions** (architecture, file relationships, where something
  lives): the `graphify` skill — a graph exists at `graphify-out/`. Fall back to
  Grep/Glob/Read only for code just modified in the current branch/session
  (graph may be stale) or when exact current line numbers are needed for an
  edit. Re-run `/graphify` after substantial changes land.
- **Go work** on `server/` (write, review, debug, test): the matching
  `golang-*` skill; `golang-how-to` orchestrates which ones to load.
- **VDR feature scoping/benchmarking**: `vdr-competitor-scan`.
- **Any chart/visualization**: `dataviz` before writing chart code.

When no installed skill matches, proceed normally — do not force one.

## Commands

### Web (run inside `web/`)

```sh
bun run dev          # Vite dev server, port 5173
bun run build        # Production build
bun run lint         # prettier --check . && eslint . (prettier runs first — order matters)
bun run format       # prettier --write .
bun run check        # svelte-kit sync && svelte-check (type-checks .svelte files)
```

### Server (run inside `server/`)

```sh
go run ./cmd/main                        # Start API server (env from configs/.env)
go test ./...                            # All tests
go test ./internal/access/service/      # Single package
go test ./internal/access/service/ -run TestName   # Single test
make migrate-up                          # goose migrations up
make migrate-create name=create_xxx      # New sequential migration
make migrate-status
make sqlc                                # Regenerate sqlc code — REQUIRED after editing any repository/query/*.sql
```

Prerequisites: `configs/.env` (gitignored, `include`d by the Makefile as shell vars), running PostgreSQL + Minio, and a Gotenberg service for document conversion.

## Server architecture

- **Domain modules** under `internal/`: `auth`, `workspace`, `access`, `invitation`, `content`. Each follows `handler/` → `service/` → `repository/`.
- **sqlc**: hand-written SQL in each module's `repository/query/*.sql` compiles (via `configs/sqlc.yaml`) into a per-domain package in `repository/sqlc/` — `authdb`, `workspacedb`, `accessdb`, `invitationdb`, `contentdb` — all checked against the single shared `migrations/` schema. `emit_interface` produces `Querier` interfaces; service tests use hand-written fake repos satisfying them (`testify` assert/require).
- **Composition root**: `cmd/main/main.go` loads config/database/storage/viewer pipeline, then `internal/app/app.go New()` wires every module; `internal/app/router.go` mounts routes.
- **Platform layer** `internal/platform/`: config, database, middleware (JWT auth + workspace membership/permission guards), oauth (Google/GitHub), otp, storage (Minio incl. multipart), token, ratelimit, watermark, convert (Gotenberg → PDF), render (Poppler PDF → PNG), response, validation, sender (mail), permission, cache, logger.
- **Secure viewer pipeline**: non-PDF uploads convert via Gotenberg to PDF, pages render via Poppler to PNG renditions, watermark burned per request. Lazy — no job queue.
- **Cross-domain transactions**: repositories expose `ExecTx`/`ExecTxTx` so one pgx transaction can span domains (e.g. invitation acceptance feeds an `InvitationConsumer` implemented in `access`).
- **Migrations**: goose, sequential numbering (`-s`), in `server/migrations/`.

## Web architecture

- **Auth pattern**: tokens live in httpOnly cookies; the browser never calls the Go API directly for authenticated requests. SvelteKit server code (`hooks.server.ts`, `$lib/server/session.ts`) injects the Bearer token, and `src/routes/api/*/+server.ts` endpoints proxy calls the client must make from the browser (uploads, viewer page PNGs for `<img>` tags).
- `$lib/server/api/` — server-only API client (base `client.ts` + one file per domain).
- `$lib/upload/queue.svelte.ts` — resumable multipart upload queue (init/part-urls/complete/abort proxied under `routes/api/content/multipart/`).
- **Route groups**: `(auth)` public auth pages, `(onboarding)` workspace creation, `(app)` authenticated workspace UI.
- **i18n**: Indonesian (`id`, default) + English (`en`) in `$lib/i18n`; UI strings go through `t()`. Server-side locale via `AsyncLocalStorage`.
- Svelte 5 **runes mode forced** project-wide (vite config). Tailwind CSS **v4** via `@tailwindcss/vite` (deliberately no PostCSS config). DaisyUI **v5**. `.npmrc` sets `engine-strict=true`.

## Domain model (VDR semantics)

- A workspace is a data room. Roles are fixed: `owner`/`admin`/`guest` — do not invent new roles.
- Guests belong to exactly one group; folder permissions are boolean flags on `folder_access` per group, resolved by walking up the folder tree.
- Root level holds folders only; a default non-deletable `General` folder (`is_default`) catches root-level drops.
- Documents are versioned; `current` version is a pointer (restore = pointer flip, `current` ≠ max version_no).

## UI design constraints (must follow)

- No generic-SaaS look: no purple gradients, cream/sand palettes, hero metrics, identical card grids.
- Flat by default; elevation only as state response (hover, modal, dropdown).
- Machine facts (IDs, hashes, timestamps) always monospace.
- WCAG AA contrast minimum; `prefers-reduced-motion` support; state animations 150–250ms only.
- Full details in `web/PRODUCT.md` and `web/DESIGN.md` — read them before substantial UI work.

## CI

`.github/workflows/` is empty — no CI pipeline yet. Lint/check/test must be run manually before committing.

## Workflow

- Display the full code in the chat before writing it to a file, not just a summary of the changes.
- For long files, display the modified blocks along with their surrounding context.
- Every discussion that produces a decision, a design direction, or a rejected
  option must be written to `brainstorm-folder/` before the session ends.
  Discussion that leaves no file is treated as unfinished work.

## Discussion notes — `brainstorm-folder/`

Work is organised into numbered phases. Exactly one phase is active at a time,
and `brainstorm-folder/` holds files for the active phase only.

```
brainstorm-folder/
  current-phase.md          # permanent. index + history. never deleted.
  phase-7-description.md    # scope of the active phase
  phase-7-a.md              # step notes, in order
  phase-7-b.md
```

Flat — no subfolders, no files for past or future phases. Create the folder if
it does not exist; it is committed, not gitignored.

### `current-phase.md`

The only permanent file. It is the answer to "where is this project". Appended
to as steps close; the Completed phases section is written at every transition.

```
# Current phase

- Active: phase 7 — <name>
- Started: YYYY-MM-DD
- Status: in progress | blocked | complete

## Steps
- [x] 7-a — <title> — <one-line outcome>
- [ ] 7-b — <title>
- [ ] 7-c — <title>

## Decisions carried forward
Decisions from the active phase that outlive it. One line each, with the
affected path.

## Completed phases
### Phase 6 — <name> (YYYY-MM-DD → YYYY-MM-DD)
What shipped, what was decided, what was deliberately deferred, what is
still owed. Written at closing time — once the phase-6 files are deleted
this paragraph is all that survives of them.
```

### `phase-N-description.md`

Written once when the phase opens, before any step file exists.

```
# Phase N — <name>

## Goal
What is true when this phase is done.

## Out of scope
What is deliberately not in this phase.

## Steps
- N-a — <title>
- N-b — <title>

## Open questions
To be resolved during the phase.
```

Steps are planned upfront but not frozen. Adding `7-d` mid-phase is fine —
record it in both this file and `current-phase.md`.

### `phase-N-x.md`

One file per step, lettered in order (`-a`, `-b`, …). More than ~8 steps means
the phase is too large; split it rather than reaching `-i`.

```
# Phase N-x — <title>

- Status: open | done
- Files touched:

## Context
Why this step exists; constraints already fixed.

## Options considered
Trade-offs per option. Name the reference product when a pattern comes from
one, e.g. "Ansarada handles this as X".

## Decision
What was chosen, or `none yet`.

## Rationale
Why — and what was rejected, on what grounds.

## Follow-ups
Next actions and the files likely to change.
```

### Phase transition

Never initiate this. It runs only after the user explicitly states the phase is
complete, and in this order:

1. Verify every `phase-N-*.md` has `Status: done`. If any does not, report which
   ones and stop.
2. Lift every durable decision out of the step files into `current-phase.md` →
   Completed phases. Write it assuming the step files are about to become
   unreadable, because they are.
3. Reflect any rule change (roles, permission semantics, domain model, UI
   constraints) into `CLAUDE.md`, `AGENTS.md`, `web/PRODUCT.md`, or
   `web/DESIGN.md`.
4. Show the proposed new `current-phase.md` in chat and wait for approval.
5. Only then delete `phase-N-description.md` and every `phase-N-*.md`.
6. Write `phase-(N+1)-description.md` and reset the Steps checklist.

Steps 2–4 happen before step 5, never after. A deletion that precedes the
summary loses the phase.

### Content rules

- Record rejected options and the reason. A note holding only the winner is
  useless three months later.
- No code dumps. Reference paths (`server/internal/access/service/`) instead of
  pasting implementations; include a snippet only when the decision *is* the
  exact shape of an API or schema.
- Mark unverified assumptions so they can be checked later.
- `brainstorm-folder/` is a working record, not a source of truth. Project rules
  live in the instruction files.

## Code conventions
- No abstraction until the third occurrence. Duplicate twice, extract on the third.
- No new dependency without stating what it replaces and why writing it is worse.
- Prefer editing an existing module over creating a new one. Say which file you
  considered and why it didn't fit.
- One function, one reason to change. If explaining it needs "and", split it.

## Project

Self-serve virtual data room SaaS for the lower-mid market: individuals,
small teams, and buyers priced out of enterprise VDRs. Transparent flat
pricing and setup without training are core positioning, not interim
constraints.

Enterprise is a later direction, not a current target. Do not scope for
enterprise requirements (SSO, procurement workflows, deep admin hierarchies)
unless explicitly asked.

The substitute to beat is a shared cloud drive with a link — not iDeals.
Ellty is the closest direct peer; iDeals, Ansarada, and Datasite are
capability references from a tier above.

## Reference products

Benchmark set for this project. When scoping, naming, or comparing a feature,
check it against these before proposing a design:

- **Ellty** — https://www.ellty.com/virtual-data-room
- **iDeals** — https://www.idealsvdr.com
- **Ansarada** — https://www.ansarada.com/data-room
- **Datasite** — https://www.datasite.com

How to use them:
- Name the source when proposing a pattern, e.g. "Ansarada handles this as X".
- Where they differ, present the options instead of silently picking one.
- Parity is not the goal. Flag competitor features that fall outside this
  project's scope rather than assuming we need them.
- Ellty targets a different tier than the other three — compare accordingly.

For a full capability scan, use the `vdr-competitor-scan` skill.

## Warning

All the code you generate will be audited by other AI models such as DeepSeek-v4 Pro, ChatGPT, Kimi K3, and GLM.

So make sure the code you generate has no flaws that these models could flag.