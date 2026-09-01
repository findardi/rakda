# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

**Rakda** (formerly Wadi, then Riksa) — a Virtual Data Room. Two-package monorepo:

- `web/` — SvelteKit 5 frontend. **Uses Bun, not npm** (`web/README.md` npm instructions are scaffold boilerplate).
- `server/` — Go 1.26 backend (Chi, PostgreSQL/pgx, Minio). Module path: `github.com/findardi/rakda/server`.
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

Prerequisites: `configs/.env` (gitignored, `include`d by the Makefile as shell vars), running PostgreSQL + Minio, and a Gotenberg service for document conversion — all three provided by `docker compose up -d` (below).

### Docker (everything lives in `docker/`; run from repo root)

```sh
docker compose -f docker/compose.yaml up -d        # Daily dev: infra only (Postgres 16, Minio, Gotenberg); API/web run on the host
docker compose -f docker/compose.full.yaml build   # Full-docker dev, prod-shaped images
docker compose -f docker/compose.full.yaml run --rm migrate up
docker compose -f docker/compose.full.yaml up -d   # Browser at http://localhost:5173 via Traefik (don't run alongside the daily stack)
docker compose -f docker/compose.prod.yaml config  # Validate prod skeleton
```

Dockerfiles are `docker/server.Dockerfile` and `docker/web.Dockerfile` (contexts stay `server/` and `web/` — the two `.dockerignore` files must remain at those context roots to work). Secrets for the full/prod stacks live in gitignored env files inside `docker/`, copied from the committed samples (`.env.full.sample`, `.env.prod.api.sample`, `.env.prod.web.sample`). Postgres is pinned to major **16** to match the managed database. In prod only api/web/gotenberg are containers: Postgres is the provider's managed database and Minio is replaced by S3-compatible object storage — both external, configured purely via env. `MINIO_ENDPOINT` is baked into browser-facing presigned upload URLs; the full-docker stack solves the two-audience problem with the `minio.localhost` network alias (never point it at a container-only hostname).

## Server architecture

- **Domain modules** under `internal/`: `auth`, `workspace`, `access`, `invitation`, `content`, `activity`, `qa`. Each follows `handler/` → `service/` → `repository/`.
- **sqlc**: hand-written SQL in each module's `repository/query/*.sql` compiles (via `configs/sqlc.yaml`) into a per-domain package in `repository/sqlc/` — `authdb`, `workspacedb`, `accessdb`, `invitationdb`, `contentdb`, `activitydb`, `qadb` — all checked against the single shared `migrations/` schema. `emit_interface` produces `Querier` interfaces; service tests use hand-written fake repos satisfying them (`testify` assert/require).
- **Composition root**: `cmd/main/main.go` loads config/database/storage/viewer pipeline, then `internal/app/app.go New()` wires every module; `internal/app/router.go` mounts routes.
- **Platform layer** `internal/platform/`: config, database, middleware (JWT auth + workspace membership/permission guards), oauth (Google/GitHub), otp, storage (Minio incl. multipart), token, ratelimit, watermark, convert (Gotenberg → PDF), render (Poppler PDF → PNG), response, validation, sender (mail), permission, cache, logger.
- **Secure viewer pipeline**: non-PDF uploads convert via Gotenberg to PDF, pages render via Poppler to PNG renditions, watermark burned per request. Lazy — no job queue. Page PNGs are cached under a top-level `page-cache/` prefix, deliberately **separate** from `renditions/`, so the age-based sweeper (`Storage.DeleteOlderThan`, 7-day TTL) can expire them without ever reaching `rendition.pdf` — which both downloads and the search pipeline depend on. Never move the cache back under `renditions/`.
- **A long watermarked download becomes a stored artifact, not a failed request.** `pageCount > 100` queues immediately; anything smaller is assembled synchronously under a **30 s budget** and *escalates* into a job if the budget runs out — the in-flight goroutine is handed over, so no work is repeated. This is possible only because `rasterWatermarkPDF` finishes the whole file before returning a `ReadCloser`: nothing is written to the response when the budget expires, so the handler can still answer **202** with a job id. A page threshold alone was rejected — cost tracks pixels × complexity, not page count (a 150-page A3 scan and a 150-page A4 text document are nowhere near each other), so the threshold is only a coarse guard and the time budget is what actually decides. **This touches the watermarked variant only**: owner/admin `bypassesContentAccess()` and always take the clean `store.Get` path, and in an `archive` room a guest's `can_download` is already forced false (403), so no `RequireRoomWritable` exception exists or is needed. Artifacts live in a top-level `downloads/` prefix with a **30-minute** TTL — per-request and never reusable, unlike a room archive. Permission is **re-checked at delivery** (`GetDownloadJobObject` calls `resolveViewAccess` again), because the artifact outlives the request that made it. The audit row is written **on delivery and only at `offset == 0`**, so a resumed transfer does not double-count and an uncollected job records nothing. A `pending` job is de-duplicated per (workspace, requester, version); a `ready` one is **never** reused — `watermark.Mark` carries a minute-precision timestamp and the client IP, so serving an old artifact would serve a mark that lies. The browser fetches it with a plain `<a href download>`; the 150-page cap stays, because async removes the timeout failure, not the cost.
- **CPU budget: the release box is 2 vCPU / 8 GB.** Every cost figure that assumed 4 vCPU is wrong and must be re-measured (`open-debts.md` U-20, U-60). Container CPU ceilings live in `docker/compose.prod.yaml` and `compose.dev-server.yaml` — gotenberg 1.0, api 1.5, web 0.5, traefik 0.25; they sum above 2 on purpose, because `cpus:` is a **per-container ceiling, not a reservation**. `compose.full.yaml` is deliberately unlimited (it also runs Postgres and MinIO locally, so it is not a mirror of the prod box).
- **The request path and the background sweepers hold separate Poppler pools, and must stay separate.** `main.go` builds two `PopplerRenderer` instances: the request one (`VIEWER_RENDER_CONCURRENCY`, default 2) serves `Viewer.Renderer`, and a niced one (`VIEWER_SWEEP_CONCURRENCY` default 1, `VIEWER_SWEEP_NICE` default 10) serves `Viewer.TextExtractor` and `Viewer.WordBoxes`. Tesseract has its own pool plus `OCR_NICE`. Merging them back puts a reader behind an OCR sweep — two loads whose deadlines have nothing in common. `nice` is not a CPU quota: it only reorders under contention, which is exactly what is wanted.
- **`render.WordBoxExtractor` and `render.OCR` are session interfaces (`OpenWordBoxes`/`OpenOCR`), never per-page.** The per-page variants were deleted, not deprecated: their only caller is a sweeper that always works page-groups of one version, and re-fetching the whole rendition per page made I/O scale with pages × document size. `groupPagesByVersion` in `content_text_service.go` keeps one `store.Get` + one spool per version while the sweeper quota stays **per page** (a 750-page scan must remain resumable mid-document). Because a spool now outlives one page, `tesseractDocument.OCRPage` must use a per-page prefix and delete its PNG.
- **Client IP**: `X-Forwarded-For` is honoured only when the immediate peer matches `TRUSTED_PROXY_CIDRS`, and the hop taken is the **rightmost one that is not itself a trusted proxy** — every proxy is trusted only for the hop it appended. Default empty means XFF is never trusted, so a misconfiguration yields a useless IP rather than a forgeable one. The value feeds both the burned watermark and the per-IP rate-limit key, so a proxy that fails to forward it silently collapses every caller into one bucket.
- **Cross-domain transactions**: repositories expose `ExecTx`/`ExecTxTx` so one pgx transaction can span domains (e.g. invitation acceptance feeds an `InvitationConsumer` implemented in `access`).
- **Audit & activity** (`activity` domain): every meaningful action writes one row to `activity_logs` — same-tx via `RecordTx(tx)` when the action already runs in `ExecTxTx`, best-effort `Record` otherwise; consumers declare an `ActivityRecorder` port in their `ports.go`. Page views (`view_page`, server-emitted) and read durations (`page_duration`, browser-beacon) go to `content_events` — append-only, `document_id` deliberately has no FK, names/actors are snapshotted at write time. **Room managers produce no `content_events` at all**: owner/admin are filtered on the *write* side (`content_view_service.go` skips `RecordPageEvent`, `RecordPageDurations` no-ops, the client builds no dwell tracker), so a guest later promoted to admin keeps their reading history — `activity_logs` still records `document_viewed` for every role. Timeline and engagement endpoints are owner/admin only (guests are recorded, never readers). Engagement is **per reader, two levels** — L1 reader list, L2 that reader's pages; there is no cross-reader page heatmap.
- **Search pipeline**: `pdftotext` fills `document_page_texts` (one row per page keyed to `version_id`, with `tsv_id`/`tsv_en` generated columns for the `indonesian` and `english` configs) via a quota'd background sweeper following the `RunReaper` shape. Pages with no text layer are OCR'd by Tesseract in a second sweeper quota'd **per page, not per version** — a 750-page scan is ~40 min of CPU, so it must be resumable mid-document; it also stores word boxes normalised to 0..1. Name search uses `pg_trgm` + `ILIKE`; content search is permission-filtered **inside the query** by reusing the `granted` CTE from `ListVisibleFolders`, which makes snippet and result-count leaks impossible by construction. Searches are audited with their keywords (`search_performed`, `target_name` = query) and written on commit via `POST /search/log` — never from the `GET`, which must stay side-effect free.
- **Migrations**: goose, sequential numbering (`-s`), in `server/migrations/`. **Migrations must stay additive (expand-contract)**: the deploy pipeline moves the schema before Watchtower swaps images (old code briefly runs on the new schema), and rollback retags images without ever running `goose down` — a destructive migration breaks both directions.

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
- **Folder templates**: five curated built-in structures (M&A due diligence, fundraising, property transaction, audit & reporting, legal & litigation) defined as Go constants in `content/service/folder_templates.go` — bilingual (the applier's locale picks id/en names, read server-side from the `rakda_locale` cookie), max 2 levels, **no number prefixes** in names (ordering via `position`; display numbers are computed client-side). `GET/POST /content/workspaces/{id}/folder-templates[/{key}/apply]`, both behind `folder:create`. Apply rides the Fase-5 bulk machinery (`runFolderTreeTx`): **additive, never destructive** — an existing same-named folder is reused and its missing subfolders are filled in (merge-descend), new folders land after existing ones, `General` is untouched. Templates carry **structure only** — never group permissions, never feature presets. Every apply writes one `template_applied` activity row (`target_name` = template name, metadata `{template, created, skipped}`), even when created = 0. The picker previews the full structure with "already exists" markers before commit; when the room already has non-default root folders the UI warns and the apply button carries the computed new-folder count — **friction informs, never blocks** (a second template on a structured room is legitimate).
- **Bulk delete** (folders & documents): `POST .../folders/bulk-delete` + `.../documents/bulk-delete` (guards `folder:delete` / `document:delete`), **atomic validate-all-first** in one tx — a foreign/missing id is 404 before anything is touched, `General` is refused, duplicate ids are deduped, and the folder variant takes `LockWorkspaceStructure`. Soft-delete to trash only (each selected folder becomes its own trash root; restore stays per-item); one `{bulk: true, count}` audit row per action. UI: a selection mode per surface (folder rail / document panel) with one counted confirmation; DnD is disabled while selecting.
- Documents are versioned; `current` version is a pointer (restore = pointer flip, `current` ≠ max version_no).
- **Content delivery is Model B: native bytes never leave the server, for anyone.** Every download is served as the PDF rendition — `can_download` gets it watermarked, `can_download_original` gets it clean. "Original" means the *unmarked rendition*, **not** the uploaded file; no flag combination and no role (owner/admin included) delivers the source bytes. Downloads stream through the API — nothing is presigned from object storage any more.
- `can_watermark` and `can_download_original` are **mutually exclusive** per group per folder (rejected in `SetFolderAccess` with 400, plus a DB check constraint): marking the screen while handing out a clean file protects nothing. The cascade `orig ⇒ download ⇒ view` and `watermark ⇒ view` still applies.
- **Uploads are gated to what can become a PDF**: the extension allowlist in `platform/convert` (PDF, Office incl. spreadsheets, images), 500 MB per file, 750 pages per rendition — refused at `InitMultipart`/`CompletedUpload` (415/413). The gate is name-based, so content that only fails at conversion is caught later, recorded on the version row (`rendition_error`, `rendition_failed_at`), returned as 422, and never retried until an owner triggers a retry.
- **Both watermarks are burned into pixels; neither is removable by a PDF tool.** The viewer marks each page PNG per request; a `can_download` download is re-assembled as a flattened raster PDF through the same `ImageWatermark.Burn`. The vector stamp that `pdfcpu watermark remove` stripped in one command is gone — `platform/watermark/pdf.go` was deleted, and there must never be a second way to watermark a PDF. The cost is deliberate and must stay visible in the UI: a watermarked download has **no text layer** (no selection, copy, Ctrl+F, or screen reader), is far larger, and is uncacheable because every mark is unique per request. `can_download_original` is unchanged — a clean, text-bearing rendition. Details in `web/PRODUCT.md`.
- **A watermarked download is capped at 150 pages** (413); the clean variant has no cap. This is a cost ceiling, not a permission: assembly holds ~10 MB of decompressed pixels per page, and the web proxy aborts at a hard 300 s that Bun cannot override (`timeout`/`headersTimeout`/`bodyTimeout` are all ignored — measured). The number is served to the client as `watermark_download_max_pages` on the view meta so the button can disable itself and explain — **never hardcode it in the web tier**. Pages are rendered and burned in parallel (2 workers), but the PDF import is batched **sequentially**, 25 pages at a time: parallelising the batches holds several batches in RAM at once, which is the exact failure this design prevents. `DOWNLOAD_STAMP_CONCURRENCY` is a **RAM multiplier** (≈ concurrency × pages × 10.35 MB), not a throughput knob — default 1; raising it without measuring is how the box OOMs. `http.Server` is deliberately configured **without `WriteTimeout`** — adding one cuts long downloads.
- The viewer's find layer sends **coordinates, not text**: the client posts a query, the server returns only the matching word boxes. Shipping a page's words to the browser would undo the reason the viewer rasterises at all — the leak is the payload, not the DOM overlay.
- **Viewer hardening is always-on and role-independent.** Right-click is suppressed on the page wrapper (`.rakda-vp`, not app-wide), `@media print` blanks the reader and prints one **adaptive** notice instead — naming the Download button only when the reader actually holds the permission — and an opaque curtain covers the reading area 500 ms after the window loses focus (re-checked at timer maturity, raised instantly, faded down). None of the three has a toggle: a hole is not given a switch, and role does not change who can see a screen. The curtain also stops the dwell clock, so engagement figures mean **"time with the window focused"**, not "time with the tab open" — figures from before 2026-08-22 are not comparable.
- **"Mode privasi" is a reader preference, not a permission.** A cursor-following band (one overlay sibling of the scroll container, position written as a CSS custom property inside rAF — never `$state`, which would re-evaluate the template at pointer rate) stored in `localStorage`, off by default. No `folder_access` flag, no audit row, no request; owners can neither force it nor see who uses it. Fence as a per-group permission was **rejected** (20 files + the `granted` CTE, which is duplicated 6×); if a forced variant is ever needed the path is a `workspaces`-level setting, never `folder_access`.
- **All screen protection is deterrence, not a control, and the UI must never claim otherwise.** No browser API blocks screenshots; Win+Shift+S freezes the screen before the `blur` event can be handled, and large headings stay guessable through the blur. The phrase **"screenshot protection" is banned** in UI copy — the feature is named "Mode privasi". Details in `web/PRODUCT.md`.
- Audit trail is two separate tables, never merged: `activity_logs` (one row per action, chronological timeline) vs `content_events` (per-page, high volume, aggregation only). Both are append-only — never UPDATE audit rows. Visible to owner/admin only; guests never see any activity, including their own. Owner/admin generate no `content_events` (filtered at write time); `activity_logs` covers every role.
- **Q&A (`qa` domain) is room-scoped and group-siloed.** Guests ask; owner/admin answer from one shared queue — no Q&A roles, no assignment, no categories, no priorities. A question belongs to (workspace, asker's group): group members see each other's threads, one group **never** sees another's, enforced by a plain `WHERE group_id` — deliberately **not** the `granted` CTE, because Q&A visibility follows groups, not folders. Status `waiting`/`answered`/`closed` is **reply-driven**: a manager reply flips to `answered`, a guest reply back to `waiting`; close = original asker or manager, reopen = manager only, replies to `closed` are rejected. Questions and replies are append-only — no edits, corrections are new replies. A question may reference a document/folder: FK `on delete set null` + name snapshot (cascade is forbidden — `documents` rows cascade when a folder is purged); the reference chip is permission-checked per reader at read time (`ContentService.CanUserViewFolder`) and silently omitted when not allowed. The **FAQ is the only cross-group channel** and is anonymous **by construction**: a `faqs` row carries no group/author, promote = an owner-editable INSERT copy, `source_question_id` is never serialized, and FAQs carry **no document reference** (it would leak names to unpermitted groups). Per-group knobs live on `workspace_groups`, never `folder_access`: `qa_enabled` (off = section hidden for that group's guests, data kept) and `qa_question_limit` (NULL = unlimited, 0 = submissions blocked but section visible; replies don't count). The limit check and the **per-group** sequential display number share one `SELECT … FOR UPDATE` on the group row (room-wide numbering was rejected — it leaks cross-group volume). Quota numbers reach the client server-computed (`quota_remaining`) — never derived or hardcoded in web. Every meaningful action writes one `activity_logs` row same-tx — `question_submitted` (metadata `{group_name, number}`), `question_replied`, `question_answered`, `question_closed`, `question_reopened`, `faq_published`, `qa_settings_changed` — with `target_name` = subject and content never included; `content_events` is untouched by Q&A. Export is CSV, multi-row per message, both sides, scoped to what the caller already sees (guest = own silo; disabled group → 403; FAQ not exported). Group settings go through `PATCH /access/workspaces/{id}/groups/{groupID}/qa` — never the full-replace group `PUT`.

- **Room lifecycle is enforced at the request path, and mutates nothing.** `workspaces.status` is `prepare` / `active` / `archive`, and rooms are born `prepare`. **`prepare`** = internal room; guests cannot enter at all (403) — this protects something real, because `GrantDefaultFolderAccess` gives the default group `can_view` on `General` the moment a room is created and every accepted guest lands in that group. **`active`** = no restrictions. **`archive`** = frozen for **every** role including owner/admin, but **readable** by every role; guests drop to view-only — `can_download` and `can_download_original` forced `false`, `can_watermark` forced `true`, all decided in the one place that already owns those flags, `resolveViewAccess`. Archiving **writes no data row**, so un-archive is a single UPDATE and the `invitation` domain is untouched. Rakda deliberately has **no "fully closed" state**: `archive` does the job of iDeals' *Archived*, not *Closed*; if one is ever needed the path is a fourth state, never a redefinition of `archive`.
- **The lifecycle gate is two `r.Use` middlewares on the HTTP method, never a permission classification.** `RequireRoomOpenForGuests` (guest + `prepare` → 403) and `RequireRoomWritable` (`archive` + non-GET/HEAD → **423 Locked**) sit in all four `{workspaceID}` modules. Room status rides the membership query `GetMembershipWithPermissions` via one primary-key join — **zero extra round-trips and zero touches to the `granted` CTE**. The axis must stay the HTTP method: permission constants are not 1:1 with mutation (`GET /folder-templates/` is behind `folder:create`, `GET .../multipart/parts` behind `document:upload`), so a permission-based gate would 423 pure reads. And the gate must never sink into service/repository/storage: `GET /view` writes (`ensureRendition`, `promoteStaged`, page-cache `store.Put`, `RecordPageEvent`), so a guard below the handler turns an archived room into an **unreadable** one. Four deliberate exceptions sit as router siblings, not per-route opt-outs: `POST /search/log` and `POST .../duration` (their only effect is an audit row), `POST .../retry-rendition` (without it a document whose conversion once failed is permanently unreadable — `ensureRendition` short-circuits on `rendition_failed_at` and only `ClearVersionRenditionFailure` clears it), and `POST .../archives` (the archive export is most needed exactly in an archived room). Status transitions are guarded (`active`/`archive` → `prepare` rejected 409, same-to-same rejected 409) and write one `workspace_status_changed` audit row with `{from, to}` metadata; rejections are never audited — a 423 is not an action.
- **Archive export is a stored artifact, not a stream.** `POST/GET/DELETE /content/workspaces/{id}/archives[/{archiveID}][/download]`, manager-only, available in **every** room status. Generation is async into the top-level `archives/` prefix — kept separate from `renditions/` for exactly the reason `page-cache/` is, so its 30-day TTL sweep can never reach `rendition.pdf`. The `workspace_archives` row (`pending`/`ready`/`failed`) is **not** a job queue: it is the same shape as `staged_version_id` + `rendition_failed_at` — one status row, one goroutine, manual retry, and a sweeper that fails rows left hanging by a deploy. Download is served with `Content-Length` + `Accept-Ranges` so an interrupted transfer **resumes**; that is the whole reason it is stored rather than streamed, because the hard 300 s proxy cap is a bandwidth cliff that no room-size limit can avoid. The browser must fetch it with a plain `<a href>`, **never `fetch`+blob** — a blob holds the entire room in tab memory. Contents are clean renditions (managers bypass content access, so nothing is watermarked and assembly stays cheap), the folder tree with cumulative dotted number prefixes baked into names (a ZIP loses `position`), a clickable `_indeks.html` plus a machine-readable `_indeks.csv`, and `_audit/` CSVs. Documents whose rendition failed are **reported, not silently dropped**: the index keeps the row with a failed status and the README states the count.
- **The room list carries role and last activity, and the quota comes from the server.** `GET /workspaces` returns `{workspaces, owned_count, owned_limit}` — the 3-room cap is **never hardcoded in the web tier**, the same rule that already governs `watermark_download_max_pages`. Rows order by `last_activity_at` (from `activity_logs`, whose `(workspace_id, created_at desc)` index already exists) because a multi-room user needs "what moved", not "when it started". The room switcher lives in the `RoomSidebar` identity block, not the topbar: this shell already swaps room context in the sidebar, and the room list is already loaded there to resolve the slug, so the switcher costs no extra request. Archived rooms stay mixed into the list with their badge — they remain readable, so they are not a trap. **The cap applies only to rooms a user OWNS**; guests can belong to unlimited rooms, and that is the real multi-room case this market has.

## UI design constraints (must follow)

- No generic-SaaS look: no purple gradients, cream/sand palettes, hero metrics, identical card grids.
- Flat by default; elevation only as state response (hover, modal, dropdown).
- Machine facts (IDs, hashes, timestamps) always monospace.
- WCAG AA contrast minimum; `prefers-reduced-motion` support; state animations 150–250ms only.
- Full details in `web/PRODUCT.md` and `web/DESIGN.md` — read them before substantial UI work.

## CI

Two workflows in `.github/workflows/`:

- **`deploy.yml`** (push to `main`): test (`go test`, `bun run check`) → build+push all three images to GHCR tagged `sha-<commit>` only → run goose migrations against the managed prod DB (secret `PROD_GOOSE_DBSTRING`; the provider must allow GitHub-runner connections) → **promote**: retag `sha-<commit>` as `:main`. The `:main` tag never moves before migrations succeed, so the server-side updater never pulls an image whose schema isn't ready.
- **`rollback.yml`** (manual, workflow_dispatch with a commit SHA): retags `:main` back to that release's `sha-<commit>`; the updater picks it up on its next run. Goose is never rolled back automatically — migrations must stay additive.
- **`deploy-dev.yml`** (push to `dev`): the same pipeline on the dev channel — migrates the **`rakda_dev`** database (secret `DEV_GOOSE_DBSTRING`; same managed instance, separate database and S3 bucket) and promotes **`:dev`**, pulled by a separate dev VPS (`docker/compose.dev-server.yaml`, subnet 172.30.0.0/24, pull-only — no `build:`, timer `rakda-dev-update`). Dev rolls forward (push again); `rollback.yml` targets `:main` only.

The prod host runs **rootful Podman, not Docker**. Rootful is mandatory: rootless bridge networking (rootlessport) discards real client source IPs, which would silently collapse the XFF chain (watermark + rate-limit IPs). The updater is a **systemd timer** (`docker/systemd/rakda-update.{service,timer}`) running `podman compose pull` + `up -d` every 5 minutes — Watchtower was dropped as a Docker-API-only tool. GHCR images are private: one-time `podman login ghcr.io` with a `read:packages` PAT. Enable `podman-restart.service` so containers come back after reboot. CI does not run eslint (U-38); lint stays manual.

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