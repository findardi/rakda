# Wadi (Virtual Data Room)

## Two-package monorepo
- `web/` — SvelteKit 5 frontend (Bun package manager)
- `server/` — Go 1.26 backend (Chi, PostgreSQL/pgx, Minio)

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

## Warning

All the code you generate will be audited by other AI models such as DeepSeek-v4 Pro, ChatGPT, Kimi K3, and GLM.

So make sure the code you generate has no flaws that these models could flag. 