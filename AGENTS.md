# Wadi (Virtual Data Room)

## Two-package monorepo
- `web/` — SvelteKit 5 frontend (Bun package manager)
- `server/` — Go 1.26 backend (Chi, PostgreSQL/pgx, Minio)
- `brainstorm-folder/` — discussion and decision notes for the active phase

`CLAUDE.md` carries the same instructions for Claude Code; keep the two in sync
when editing either.

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

## Web (`web/`)

### Dev commands
```sh
bun run dev          # Start dev server (Vite, port 5173)
bun run build        # Production build
bun run lint         # prettier --check . && eslint .
bun run format       # prettier --write .
bun run check        # svelte-kit sync && svelte-check (type-check .svelte files)
```

### Stack notes
- Svelte 5 **runes mode forced** project-wide (see vite config)
- Tailwind CSS **v4** via `@tailwindcss/vite` plugin (no PostCSS config)
- DaisyUI **v5** for components
- i18n: Indonesian (`id`) + English (`en`), default `id`. Server locale via `AsyncLocalStorage`.
- `.npmrc` sets `engine-strict=true` — install fails if Node/bun version mismatches.
- Lint order matters: run `bun run lint` which runs prettier first, then eslint.

## Server (`server/`)

### Dev commands
```sh
go run ./cmd/main              # Starts API server (env from configs/.env)
make migrate-up                # Run PostgreSQL migrations (goose)
make migrate-create name=xxx   # Create a new migration
make sqlc                      # Regenerate type-safe SQL code (sqlc)
go test ./...                  # Run all tests
```

### Prerequisites
- `configs/.env` is **gitignored** but required. Copy from example or create manually.
- `configs/.env` is `include`d by the Makefile as if sourcing shell vars.
- Requires running PostgreSQL + Minio instance.
- Gotenberg service for document conversion at runtime.

### Architecture
- Modules: `auth`, `workspace`, `access`, `invitation`, `content` under `internal/`.
- Each module follows: `handler/` → `service/` → `repository/` (sqlc-generated queries).
- Shared platform layer at `internal/platform/`: config, database, middleware, oauth, otp, storage, token, etc.
- 5 separate sqlc packages in `sqlc.yaml` — one per domain (authdb, workspacedb, accessdb, invitationdb, contentdb).
- **After changing SQL queries** in any `repository/query/` directory, run `make sqlc` to regenerate.

### Tests
- Use `testify/assert` and `testify/require`.
- Service tests use fake repos satisfying generated sqlc interfaces.
- Existing tests: `access/service/`, `platform/middleware/`, `platform/watermark/`.

## Design constraints (must follow in UI code)
- No generic-SaaS look (no purple gradients, cream/sand, hero-metric, identical card grids).
- Flat by default; elevation only for state response (hover, modal, dropdown).
- Machine facts (IDs, hashes, timestamps) always in monospace font.
- WCAG AA contrast minimum. `prefers-reduced-motion` support. Only 150–250ms state animation.
- Full details in `web/PRODUCT.md` and `web/DESIGN.md`.

## CI
- `.github/workflows/` is currently empty — no CI pipeline yet.

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

## Warning

All the code you generate will be audited by other AI models such as DeepSeek-v4 Pro, ChatGPT, Kimi K3, and GLM.

So make sure the code you generate has no flaws that these models could flag.