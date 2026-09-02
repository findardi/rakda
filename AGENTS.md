# Rakda (Virtual Data Room)

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
- All three infra services are provided by `docker compose up -d` at the repo root.

### Docker stacks (everything in `docker/`; run from repo root)
```sh
docker compose -f docker/compose.yaml up -d        # Daily dev: infra only (Postgres 16, Minio, Gotenberg); API/web run on the host
docker compose -f docker/compose.full.yaml build   # Full-docker dev, prod-shaped images
docker compose -f docker/compose.full.yaml run --rm migrate up
docker compose -f docker/compose.full.yaml up -d   # Browser at http://localhost:5173 via Traefik (don't run alongside the daily stack)
docker compose -f docker/compose.prod.yaml config  # Validate prod skeleton
```
Dockerfiles are `docker/server.Dockerfile` and `docker/web.Dockerfile` (contexts stay `server/` and `web/` — the two `.dockerignore` files must remain at those context roots to work). Secrets for the full/prod stacks live in gitignored env files inside `docker/`, copied from the committed samples (`.env.full.sample`, `.env.prod.api.sample`, `.env.prod.web.sample`). Postgres is pinned to major **16** to match the managed database. In prod only api/web/gotenberg are containers: Postgres is the provider's managed database and Minio is replaced by S3-compatible object storage — both external, configured purely via env. `MINIO_ENDPOINT` is baked into browser-facing presigned upload URLs; the full-docker stack solves the two-audience problem with the `minio.localhost` network alias (never point it at a container-only hostname).

### Architecture
- Modules: `auth`, `workspace`, `access`, `invitation`, `content`, `activity`, `qa` under `internal/`.
- Each module follows: `handler/` → `service/` → `repository/` (sqlc-generated queries).
- Shared platform layer at `internal/platform/`: config, database, middleware, oauth, otp, storage, token, etc.
- 7 separate sqlc packages in `sqlc.yaml` — one per domain (authdb, workspacedb, accessdb, invitationdb, contentdb, activitydb, qadb).
- Audit (`activity` domain): actions → `activity_logs` (same-tx `RecordTx` inside `ExecTxTx`, else best-effort `Record`; consumers declare an `ActivityRecorder` port); page views/read durations → `content_events` (append-only, no FK to documents, snapshotted names/actors). Two tables, never merged, never UPDATEd. Owner/admin produce **no** `content_events` — filtered on the write side (`content_view_service.go` skips `RecordPageEvent`, `RecordPageDurations` no-ops, client builds no dwell tracker) so a promoted guest keeps their history; `activity_logs` still covers every role. Timeline/engagement endpoints are owner/admin only — guests are recorded, never readers. Engagement is per-reader, two levels (L1 readers → L2 that reader's pages); no cross-reader page heatmap.
- **Q&A (`qa` domain) is room-scoped and group-siloed.** Guests ask; owner/admin answer from one shared queue — no Q&A roles, assignment, categories, or priorities. A question belongs to (workspace, asker's group): group members see each other's threads, one group never sees another's — a plain `WHERE group_id`, deliberately **not** the `granted` CTE (Q&A visibility follows groups, not folders). Status `waiting`/`answered`/`closed` is reply-driven (manager reply → `answered`, guest reply → `waiting`); close = original asker or manager, reopen = manager only, replies to `closed` rejected. Append-only — no edits; corrections are new replies. Optional document/folder reference: FK `on delete set null` + name snapshot (cascade forbidden — `documents` cascade on folder purge); the chip is permission-checked per reader (`ContentService.CanUserViewFolder`) and omitted when not allowed. FAQ = the only cross-group channel, anonymous **by construction** (row carries no group/author; promote = editable INSERT copy; `source_question_id` never serialized; no document reference — it would leak names). Per-group knobs on `workspace_groups`, never `folder_access`: `qa_enabled` (off = section hidden for that group's guests, data kept) and `qa_question_limit` (NULL = unlimited, 0 = submissions blocked but visible; replies don't count) — limit check + per-group sequential number share one `SELECT … FOR UPDATE` on the group row (room-wide numbering leaks cross-group volume). Quota numbers are server-computed (`quota_remaining`) — never derived/hardcoded in web. Every action writes one same-tx `activity_logs` row (`question_submitted` `{group_name, number}` / `question_replied` / `question_answered` / `question_closed` / `question_reopened` / `faq_published` / `qa_settings_changed`; `target_name` = subject, content never included); `content_events` untouched. Export = CSV multi-row per message, both sides, scoped to caller visibility (guest = own silo; disabled group → 403; FAQ not exported). Group settings via `PATCH /access/workspaces/{id}/groups/{groupID}/qa` — never the full-replace group `PUT`.
- **Folder templates**: five curated built-in structures (M&A DD, fundraising, property, audit, legal) as Go constants in `content/service/folder_templates.go` — bilingual (applier's locale, read server-side from the `rakda_locale` cookie), max 2 levels, no number prefixes (ordering via `position`, display numbers computed client-side). `GET/POST /content/workspaces/{id}/folder-templates[/{key}/apply]` behind `folder:create`. Apply rides the bulk machinery (`runFolderTreeTx`): additive/never destructive — existing same-named folders are reused and missing subfolders filled in (merge-descend); `General` untouched. Structure only — never group permissions or feature presets. One `template_applied` activity row per apply (metadata `{template, created, skipped}`, written even when created = 0). Picker previews with "already exists" markers; a structured room gets a warning + a counted apply button — friction informs, never blocks.
- **Bulk delete** (folders & documents): `POST .../{folders,documents}/bulk-delete` (guards `folder:delete`/`document:delete`), atomic validate-all-first in one tx (foreign id → 404 untouched; `General` refused; ids deduped; folder variant takes `LockWorkspaceStructure`); soft-delete to trash only, one `{bulk, count}` audit row per action; selection-mode UI per surface with one counted confirmation, DnD disabled while selecting.
- Content delivery = **Model B**: native bytes never leave, for anyone. Downloads stream the PDF rendition through the API (no presigned object URLs): `can_download` → watermarked, `can_download_original` → clean rendition (**not** the source file; owner/admin included). `can_watermark` and `can_download_original` are mutually exclusive per group/folder (400 + DB check constraint); cascade `orig ⇒ download ⇒ view`, `watermark ⇒ view` unchanged. Uploads gated to convertible types (`platform/convert` allowlist incl. spreadsheets), 500 MB/file, 750 pages/rendition — 415/413 at `InitMultipart`/`CompletedUpload`. Conversion failures are stored per version (`rendition_error`, `rendition_failed_at`), served as 422, and not retried until an owner asks. **Both watermarks are burned into pixels and neither is strippable**: one burn, two containers — `ImageWatermark.BurnImage` marks the page; the viewer wraps it as PNG per request, a `can_download` download wraps it as JPEG (`downloadJPEGQuality` 80) and re-assembles a flattened raster PDF. pdfcpu embeds JPEG verbatim as `DCTDecode` (72-page import 9.8 s → 0.09 s, no decoded RGB in RAM); PNG it decodes and re-deflates. Dense text measured 746 → 482 KB/page at q80 (dev laptop); the bulk of the size is rasterisation + the burned mark, so DPI/quality/`stampPagesPerRun` are the remaining levers (U-62). Never feed pdfcpu PNG again (16-h). The pdfcpu vector stamp was deleted (`platform/watermark/pdf.go`) — never add a second way to watermark a PDF. Cost, kept visible in the UI: a watermarked download has **no text layer** (no selection/copy/Ctrl+F/screen reader), is lossy and larger than the clean rendition, and is uncacheable. `can_download_original` unchanged — clean, text-bearing. **Watermarked downloads are capped at the rendition ceiling, 750 pages** (clean variant uncapped) — 150 until 16-g, when async assembly removed the hard 300 s Bun proxy abort that set it. Still reaches the client as `watermark_download_max_pages` on the view meta — never hardcode it in web. Pages render+burn in parallel (2 workers); PDF import is batched **sequentially** 25 at a time — 25 was sized for ~10.35 MB of decoded pixels per page in pdfcpu; since 16-h it holds only JPEG bytes, so `stampPagesPerRun` and the `DOWNLOAD_STAMP_CONCURRENCY` RAM multiplier are unmeasured on the target box (U-62); the burn stage still holds decoded pixels per worker, so concurrency stays a RAM knob, default 1. `http.Server` deliberately has **no `WriteTimeout`**.
- Page PNGs cache under a top-level `page-cache/` prefix, **separate** from `renditions/`, so the age sweeper (`Storage.DeleteOlderThan`, 7-day TTL) expires them without ever reaching `rendition.pdf`. `X-Forwarded-For` is honoured only from `TRUSTED_PROXY_CIDRS`, taking the **rightmost non-trusted hop**; empty default = never trusted. That value feeds both the burned watermark and the per-IP rate-limit key.
- **At-rest encryption = SSE at the object store, never in the app.** Presigned PUT means the Go process never sees upload bytes, and a PDF's rendition *is* the uploaded object (`content_view_service.go:221`), so app-side encryption cannot reach the source. `EnsureEncryption` (best-effort) + `EncryptionStatus` (decides) live on `*MinioStorage`, not the interface; `MINIO_REQUIRE_ENCRYPTION=true` = boot requirement, "cannot determine" = failure, requires `MINIO_SSL_MODE=true`. Key held by the provider (Biznet Gio NEO) — never imply more. `document_page_texts` stays plaintext (V2 designed, skipped — U-55). VPS disk is unencrypted: document content that persists on local disk (Fase 17 `platform/diskcache`) must be ciphertext under an app-held key; transient `TMPDIR` spools stay plaintext.
- **Local disk = cache + spool, never source of truth (Fase 17).** `platform/diskcache`: three tiers under `DISK_CACHE_DIR` (`renditions`/`pages`/`downloads`, byte budgets + `DISK_CACHE_MIN_FREE` — unmeasured, U-69), built once in `cmd/main`, shared with both `ContentService` instances via `CacheDeps` (two `*Cache` on one dir = two blind indexes). Empty DIR = off; every tier fail-open to S3; nil `*Cache` is a valid disabled receiver. AES-256-GCM STREAM chunks of 1 MiB (counter‖last-flag nonce, header as AAD, per-file HKDF key from `DISK_CACHE_KEY`); any tampering fails auth, the entry self-removes, that one request fails — no mid-stream fallback. Plaintext size is derived from file size, so `Reader` is `io.ReadSeekCloser` and `Range` needs no second format. Budget enforced synchronously at commit (LRU to 90 %) — no sweep interval env; page idle-TTL rides `RunPageCacheSweeper`, download TTL rides `RunDownloadJobSweeper`, `reapOnce` calls `renditions.Sweep(0)` only for free-space pressure. Tiering is explicit at the call site (`grep renditionGet`), never a decorator on `storage.Storage` (PDF rendition key *is* the original key). `renditionGet` = read-through with a custom reader, not `io.TeeReader` (cache-write failure drops the writer, commit only after EOF); `buildRendition` writes through. Download artifacts land in `downloads` under the same `object_key`; delivery checks local then `store.Stat` — `storage.ErrNotFound` ⇒ ready job `failed` + **410**, other errors stay 500. Changed `DISK_CACHE_KEY` wipes the cache at boot.
- **Long watermarked downloads become stored artifacts.** `pageCount > 100` → queued immediately; smaller → assembled synchronously under a **30 s budget**, escalating to a job (the running goroutine is handed over, nothing repeats) and answering **202** with a job id. Works because `rasterWatermarkPDF` completes the file before returning a reader, so no byte is written when the budget expires. A page threshold alone was rejected: cost tracks pixels × complexity, not pages. **Watermarked variant only** — owner/admin bypass content access and take the clean `store.Get` path, and archived-room guests are already 403, so no `RequireRoomWritable` exception. Artifacts sit in a top-level `downloads/` prefix, **30-minute** TTL. Permission is re-checked at delivery; the audit row is written on delivery and only at `offset == 0`. `pending` jobs de-duplicate per (workspace, requester, version); `ready` ones never do — `watermark.Mark` carries a timestamp and client IP. Fetched with a plain `<a href download>` + `Accept-Ranges`. **No separate page cap since 16-g** — `maxWatermarkDownloadPages = maxRenditionPages` (750), so the button never greys out on page count. Async holds its own `stampAsyncSem` (`DOWNLOAD_STAMP_ASYNC_CONCURRENCY`, default 2), separate from sync `stampSem` (1); an escalated job keeps the sync slot it already holds. `downloadJobStaleAge` is an expression (`downloadJobTimeout + downloadJobStoreTimeout + 5m`) so the hung-job sweeper can never outlive a running job. The 30-minute TTL is stamped at `ready`, not at row creation. Both job paths give the store phase its own `downloadJobStoreTimeout` context from a detached parent — the async path once shared the raster's 45-min `jobCtx`, leaving rows `pending` until the stale sweeper. `runDownloadJob` takes both timeouts as parameters (test seam only; production passes the constants; `downloadJobStaleAge` > their sum, worst case 55 min). Async jobs render on `Viewer.DownloadJobRenderer` (niced); sync and escalated stay on `Viewer.Renderer`.
- **CPU budget: release box is 4 vCPU / 4 GB (decided 2026-09-02; was 2 vCPU / 8 GB for a day, assumed 4/4 before).** Every costly stage (render, burn, Gotenberg, OCR, cache AES) is CPU-bound and Postgres + object storage are external, so cores beat memory here. **RAM is the scarce resource**: keep `mem_limit` (api 2g, gotenberg 1g), the 2 GB swapfile is an OOM net only, `DOWNLOAD_STAMP_ASYNC_CONCURRENCY` stays a RAM multiplier. CPU ceilings (`compose.prod.yaml` + `compose.dev-server.yaml`: gotenberg 1.0, api 1.5, web 0.5, traefik 0.25 = 3.25) now fit the cores; still ceilings, not reservations; none moves before a box measurement (U-20, U-60, U-62, U-70 — all figures so far are dev-laptop or 2 vCPU extrapolations). `compose.full.yaml` stays unlimited — it also runs Postgres+MinIO, so it mirrors nothing.
- **Request path, sweepers, and download jobs hold separate Poppler pools — keep them separate.** `main.go` builds three `PopplerRenderer`s: request (`VIEWER_RENDER_CONCURRENCY`, default 2) → `Viewer.Renderer`; niced sweeper (`VIEWER_SWEEP_CONCURRENCY` 1, `VIEWER_SWEEP_NICE` 10) → `Viewer.TextExtractor` + `Viewer.WordBoxes`; niced download (`VIEWER_DOWNLOAD_CONCURRENCY` 2, `VIEWER_DOWNLOAD_NICE` 10) → `Viewer.DownloadJobRenderer`, async watermarked jobs only. Tesseract keeps its own pool plus `OCR_NICE`. Merging them puts a reader behind an OCR sweep or a 750-page job; sharing the sweep pool was rejected (its one slot also runs whole-rendition `pdftotext`). `nice` reorders under contention only — that is the point, not a quota. Nil `DownloadJobRenderer` falls back to `Renderer`; `main.go` logs the three pools at boot. Sizes unmeasured on 2 vCPU (U-70).
- **`render.WordBoxExtractor` / `render.OCR` are session interfaces (`OpenWordBoxes`/`OpenOCR`), never per-page.** Per-page variants were deleted: the only caller is a sweeper working page-groups of one version, and re-fetching the rendition per page made I/O scale with pages × document size. `groupPagesByVersion` keeps one `store.Get` + one spool per version; the sweeper quota stays **per page** (750-page scans must resume mid-document). A spool now outlives one page, so `tesseractDocument.OCRPage` needs a per-page prefix and must delete its PNG.
- Search: `pdftotext` fills `document_page_texts` (per page, keyed to `version_id`; `tsv_id`/`tsv_en` generated columns for the `indonesian` + `english` configs) via a quota'd sweeper shaped like `RunReaper`. Text-less pages go to a Tesseract sweeper quota'd **per page, not per version** (a 750-page scan is ~40 min CPU, so it must resume mid-document) which also stores 0..1-normalised word boxes. Names use `pg_trgm` + `ILIKE`; content search filters permissions **inside the query** by reusing the `granted` CTE from `ListVisibleFolders`, so snippet and result-count leaks are impossible by construction. The viewer find layer returns **boxes, not text** — the leak is the payload, not the overlay. Searches are audited with keywords (`search_performed`, `target_name` = query) on commit via `POST /search/log`; the `GET` stays side-effect free.
- **Viewer hardening (Fase 10) is always-on and role-independent**: right-click suppressed on `.rakda-vp` (not app-wide), `@media print` blanks the reader via a `rakda-print-gate` class on `<html>` + a `:has()` rule in `layout.css` and prints one **adaptive** notice (names the Download button only when the reader holds the permission), and an opaque curtain covers the reading area 500 ms after focus loss (re-checked at timer maturity; raised instantly, `out:fade` only). No toggles — a hole is not given a switch. The curtain also feeds `dwell.setPage(null)`, so engagement means **"time with the window focused"**; pre-2026-08-22 figures are not comparable. **"Mode privasi"** — a cursor-following band, one overlay sibling of the scroll container, position written as a CSS custom property inside rAF (**never `$state`**), `pointerType` `mouse|pen` only, off by default, persisted in `localStorage` — is a **reader preference, not a permission**: no `folder_access` flag, no audit row, owners can neither force nor observe it. Fence as a per-group flag was rejected (20 files + the `granted` CTE, duplicated **6×**); a forced variant would be a `workspaces`-level setting, never `folder_access`. All of it is **deterrence, not a control**: no browser API blocks screenshots, and the phrase "screenshot protection" is **banned** in UI copy. The band is the one sanctioned `backdrop-filter` exception to the no-glassmorphism rule and must keep its opaque fallback.
- **After changing SQL queries** in any `repository/query/` directory, run `make sqlc` to regenerate.

### Tests
- Use `testify/assert` and `testify/require`.
- Service tests use fake repos satisfying generated sqlc interfaces.
- Existing tests: `access/service/`, `platform/middleware/`, `platform/watermark/`.

- **Room lifecycle enforced at the request path, mutating nothing.** `workspaces.status` = `prepare`/`active`/`archive`; rooms are born `prepare`. `prepare` = guests cannot enter (403) — real protection, since the default group holds `can_view` on `General` from creation and every accepted guest joins it. `active` = unrestricted. `archive` = frozen for **every** role incl. owner/admin but **readable** by all; guests drop to view-only (`can_download`/`can_download_original` forced false, `can_watermark` forced true) in `resolveViewAccess`, the one place that already owns those flags. Archiving writes **no data row** ⇒ un-archive is one UPDATE, `invitation` untouched. No "fully closed" state exists by design — `archive` is iDeals' *Archived*, not *Closed*.
- **Gate = two `r.Use` middlewares on HTTP method, never a permission classification.** `RequireRoomOpenForGuests` (guest+`prepare` → 403) and `RequireRoomWritable` (`archive`+non-GET → **423**) in all four `{workspaceID}` modules; status rides `GetMembershipWithPermissions` via one PK join (zero extra queries, zero `granted` CTE edits). Method, not permission: `GET /folder-templates/` sits behind `folder:create` and `GET .../multipart/parts` behind `document:upload`, so a permission gate would 423 pure reads. Never push the guard below the handler — `GET /view` writes (`ensureRendition`, `promoteStaged`, page cache, `RecordPageEvent`), so a lower guard makes archived rooms unreadable. Four router-sibling exceptions: `POST /search/log`, `POST .../duration`, `POST .../retry-rendition` (else a once-failed rendition is permanently unreadable), `POST .../archives`. Transitions guarded (→`prepare` and same-to-same → 409) and audited once as `workspace_status_changed` `{from,to}`; rejections never audited.
- **Archive export = stored artifact, not a stream.** Manager-only, every room status, async into the top-level `archives/` prefix (separate from `renditions/` so its 30-day TTL can never reach `rendition.pdf`). `workspace_archives` (`pending`/`ready`/`failed`) is not a job queue — same shape as `staged_version_id`+`rendition_failed_at`: one status row, one goroutine, manual retry, sweeper fails rows a deploy left hanging. Download sends `Content-Length` + `Accept-Ranges` so interrupted transfers **resume** — the reason it is stored at all, since the 300 s proxy cap is a bandwidth cliff. Browser must use a plain `<a href>`, never `fetch`+blob. Contents: clean renditions, folder tree with cumulative dotted number prefixes baked into names, clickable `_indeks.html` + machine-readable `_indeks.csv`, `_audit/` CSVs. Failed renditions are reported in the index, never silently dropped.
- **Room list carries role + last activity; quota comes from the server.** `GET /workspaces` → `{workspaces, owned_count, owned_limit}`; the 3-room cap is **never hardcoded in web** (same rule as `watermark_download_max_pages`). Ordered by `last_activity_at`. Switcher lives in the `RoomSidebar` identity block, not the topbar — the shell already swaps room context there and the list is already loaded, so it costs no extra request. Archived rooms stay mixed in with their badge (still readable ⇒ not a trap). **The cap applies only to OWNED rooms** — guests can be in unlimited rooms, which is the real multi-room case here.

## Design constraints (must follow in UI code)
- No generic-SaaS look (no purple gradients, cream/sand, hero-metric, identical card grids).
- Flat by default; elevation only for state response (hover, modal, dropdown).
- Machine facts (IDs, hashes, timestamps) always in monospace font.
- WCAG AA contrast minimum. `prefers-reduced-motion` support. Only 150–250ms state animation.
- Full details in `web/PRODUCT.md` and `web/DESIGN.md`.

## CI
- `.github/workflows/deploy.yml` (push to `main`): test → build+push GHCR images tagged `sha-<commit>` → goose migrate to managed PG (secret `PROD_GOOSE_DBSTRING`) → promote `:main`. `:main` never moves before migrations succeed. `rollback.yml` (manual) retags `:main` to an older `sha-<commit>`; goose is never auto-rolled-back — migrations must stay additive. The prod host runs **rootful Podman** (rootless discards real client IPs via rootlessport — breaks the XFF chain); updater = systemd timer in `docker/systemd/` running `podman compose pull` + `up -d` every 5 min (Watchtower dropped, Docker-API-only). The server holds **no source checkout and no git** — `/opt/rakda/docker/` is deploy artifacts only (compose, `traefik/`, env, `systemd/`) copied from the laptop with `scp`; env is edited in the repo's gitignored copies and copied one-way (U-67). Never put `git` in a server runbook. GHCR is private: one-time `podman login ghcr.io` with a `read:packages` PAT; enable `podman-restart.service` for reboot persistence. **Host disk hygiene (17-a):** the updater's third `ExecStart` is `podman image prune -f` (dangling only — rollback re-pulls from GHCR); container logs are forced to `journald` in both server composes (`x-logging`) and capped at 1 GB by `docker/systemd/journald-rakda.conf`; `api` spools (`rakda-view-*`, `rakda-rendition-*`, `rakda-wm-*`) live on the host bind mount `/srv/rakda/spool` via `TMPDIR` set **in compose, never in the env file**, and `gotenberg` spools on `/srv/rakda/gotenberg-tmp:/tmp` (uid 1001) with a self-sweep on container start and a `test -w /tmp || exit 1` guard — rootful podman auto-creates a missing bind-mount source as root-owned, and without the guard a forgotten `chown` would fail every conversion silently into `rendition_failed_at` instead of failing the boot. `platform/spool` owns the `rakda-` prefix (`spool.Prefix` — every new spool must use it), refuses boot if `TMPDIR` is not writable (`CheckWritable`), and sweeps orphans before anything else runs (`SweepOrphans`) — boot-time, not age-based, because compose never runs two `api` at once so every leftover is provably orphaned. The two host dirs must be created and `chown`ed (10001 / 1001) before the first `up -d`. `deploy-dev.yml` (push to `dev`) mirrors the pipeline on the dev channel: migrates `rakda_dev` (secret `DEV_GOOSE_DBSTRING`, same managed instance) and promotes `:dev` for the separate dev VPS (`docker/compose.dev-server.yaml`, pull-only, timer `rakda-dev-update`); dev rolls forward — `rollback.yml` targets `:main` only. CI does not run eslint (U-38); lint stays manual.

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
  open-debts.md             # permanent. debt & finding register. never deleted.
  phase-9-description.md    # scope of the active phase
  phase-9-a.md              # step notes, in order
  phase-9-b.md
```

Flat — no subfolders, no files for past or future phases. Create the folder if it
does not exist. It is **gitignored** (`.gitignore:3`, and `web/PHASE.md` via
`web/.gitignore:2`) — deliberately: these notes stay local and are never pushed.

### `current-phase.md`

One of two permanent files. It is the answer to "where is this project". Appended
to as steps close; the Completed phases section is written at every transition.

```
# Current phase

- Active: phase 9 — <name>
- Started: YYYY-MM-DD
- Status: in progress | blocked | complete

## Steps
- [x] 9-a — <title> — <one-line outcome>
- [ ] 9-b — <title>
- [ ] 9-c — <title>

## Decisions carried forward
Decisions from the active phase that outlive it. One line each, with the
affected path.

## Completed phases
### Phase 8 — <name> (YYYY-MM-DD → YYYY-MM-DD)
What shipped, what was decided, what was deliberately deferred, what is
still owed. Written at closing time — once the phase-8 files are deleted
this paragraph is all that survives of them.
```


### `open-debts.md`

The second permanent file, and the only one not tied to a phase. Every debt,
known bug, wrong claim in the docs, and unmeasured assumption lives here — with
`path:line` evidence, so it can be verified months later.

Why it exists: debt used to be scattered across per-phase summaries and step
files that get **deleted** at transition, so old debt had to be rediscovered by
reading the whole history, and findings made while researching something else had
no home at all.

```
### U-07 — one-line title
`open` · `needs decision` · where it was found

Two or three sentences with path:line evidence and the concrete consequence.
```

Rules:

- **Adding**: use the next unused ID; never recycle a retired one. An entry
  without evidence cannot be verified later.
- **Closing**: move it to the **Lunas** (settled) section with how it was closed.
  Never delete — that settled list is what stops the same debt being rediscovered.
- **Status**: `terbuka` (open) · `butuh keputusan` (waiting on the user, not on an
  engineer) · `diterima` (deliberately accepted, not an oversight) · `lunas`.
- Debt with a natural home in a phase still goes here, with a pointer to that
  phase. This file is the index; it does not compete with the phase notes.

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

Steps are planned upfront but not frozen. Adding `9-d` mid-phase is fine —
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