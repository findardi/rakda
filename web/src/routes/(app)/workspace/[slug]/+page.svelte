<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Field, TextareaField, Toaster, showToast } from '$lib/components/common';
	import { WorkspaceStatusBadge } from '$lib/components/app';
	import {
		canDeleteWorkspace,
		canEditWorkspace,
		canManageAccess,
		canTransitionRoom,
		isRoomOpenTo,
		isRoomReadOnly
	} from '$lib/access/roles';
	import { describeActivity } from '$lib/activity/describe';
	import { formatBytes } from '$lib/format';
	import { t } from '$lib/i18n';
	import { readRecents, type RecentDocument, type RecentFolder } from '$lib/recents';
	import type { ActivityItem } from '$lib/types/activity';
	import type { ArchiveData } from '$lib/types/archive';
	import type {
		MyAccessWorkspace,
		WorkspaceStatus,
		WorkspaceSummaryData
	} from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const ws = $derived(data.workspace);

	// Edit, status, and delete are all RequireOwner on the backend — owner only.
	const role = $derived((data as { access?: MyAccessWorkspace }).access?.role ?? '');
	const canEdit = $derived(canEditWorkspace(role));
	const canDelete = $derived(canDeleteWorkspace(role));

	// One source for the gate: `roomStatus` comes from the same membership query
	// the server guards read, so the UI can never offer what the API refuses.
	const status = $derived((data as { roomStatus?: WorkspaceStatus }).roomStatus ?? ws.status);
	const roomOpen = $derived(isRoomOpenTo(status, role));
	const readOnly = $derived(isRoomReadOnly(status));
	const guestCount = $derived((data as { guestCount?: number }).guestCount ?? 0);

	// Summary, activity strip, and archive exports are manager-only on the
	// backend; mirror that here.
	const managesRoom = $derived(canManageAccess(role));
	const summary = $derived((data as { summary?: WorkspaceSummaryData | null }).summary ?? null);
	const recentActivity = $derived(
		(data as { recentActivity?: ActivityItem[] }).recentActivity ?? []
	);
	const archives = $derived((data as { archives?: ArchiveData[] }).archives ?? []);
	const archivePending = $derived(archives.some((a) => a.status === 'pending'));

	// Hero identity: deterministic from the slug so every room keeps one face —
	// the default until per-room branding images exist.
	const heroSeed = $derived.by(() => {
		let h = 0;
		for (const c of ws.slug) h = (h * 31 + c.charCodeAt(0)) >>> 0;
		return h;
	});
	const heroColor = $derived(`oklch(0.45 0.07 ${190 + (heroSeed % 5) * 12})`);
	const heroPhase = $derived(heroSeed % 40);
	const monogram = $derived.by(() => {
		const words = ws.name.trim().split(/\s+/);
		const a = words[0]?.[0] ?? '';
		const b = words.length > 1 ? (words[words.length - 1]?.[0] ?? '') : (words[0]?.[1] ?? '');
		return (a + b).toLocaleUpperCase();
	});

	const documentsHref = $derived(resolve('/(app)/workspace/[slug]/document', { slug: ws.slug }));
	const membersHref = $derived(
		resolve('/(app)/workspace/[slug]/management-access/member', { slug: ws.slug })
	);
	const inviteHref = $derived(
		resolve('/(app)/workspace/[slug]/management-access/member/invite', { slug: ws.slug })
	);
	const activityHref = $derived(resolve('/(app)/workspace/[slug]/activity', { slug: ws.slug }));
	const folderHref = (id: string) =>
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug: ws.slug, folderId: id });
	const docHref = (d: RecentDocument) =>
		resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
			slug: ws.slug,
			folderId: d.folderId,
			documentId: d.id
		});

	// Per-device history, read on the client only — see $lib/recents.ts.
	let recentFolders = $state<RecentFolder[]>([]);
	let recentDocs = $state<RecentDocument[]>([]);
	onMount(() => {
		if (managesRoom) return;
		const r = readRecents(ws.id);
		recentFolders = r.folders;
		recentDocs = r.documents;
	});

	// Assembly runs for minutes in a goroutine with no socket attached, so the
	// only way the page learns it finished is to ask again.
	$effect(() => {
		if (!archivePending) return;
		const timer = setInterval(() => {
			if (document.visibilityState === 'visible') void invalidateAll();
		}, 10_000);
		return () => clearInterval(timer);
	});

	let exportSubmitting = $state(false);
	const submitExport: SubmitFunction = () => {
		exportSubmitting = true;
		return async ({ result }) => {
			exportSubmitting = false;
			if (result.type === 'success') {
				await invalidateAll();
				showToast(t('archive.queued'), 'success');
			} else if (result.type === 'failure') {
				showToast((result.data?.message as string) ?? t('err.generic'), 'error');
			} else {
				showToast(t('err.generic'), 'error');
			}
		};
	};

	let archiveDeleteDialog = $state<HTMLDialogElement>();
	let archiveDeleteTarget = $state<ArchiveData | null>(null);
	let archiveDeleteMessage = $state<string | null>(null);
	let archiveDeleteSubmitting = $state(false);

	function openArchiveDelete(a: ArchiveData) {
		archiveDeleteTarget = a;
		archiveDeleteMessage = null;
		archiveDeleteDialog?.showModal();
	}

	const submitArchiveDelete: SubmitFunction = () => {
		archiveDeleteSubmitting = true;
		return async ({ result }) => {
			archiveDeleteSubmitting = false;
			if (result.type === 'success') {
				archiveDeleteDialog?.close();
				await invalidateAll();
			} else if (result.type === 'failure') {
				archiveDeleteMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				archiveDeleteMessage = t('err.generic');
			}
		};
	};

	const archiveHref = (id: string) =>
		`/api/content/archives/${id}/download?workspaceId=${encodeURIComponent(ws.id)}`;

	const dateFmt = new Intl.DateTimeFormat('id-ID', {
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	});
	const fmtDate = (iso: string) => dateFmt.format(new Date(iso));
	const fmtStamp = (at: number) => dateFmt.format(new Date(at));

	const statusHint = $derived(t(`ws.status.hint.${status}`));

	// --- Status change ---
	let pendingStatus = $state<WorkspaceStatus | null>(null);
	let archiveDialog = $state<HTMLDialogElement>();
	let archiveMessage = $state<string | null>(null);
	let activateDialog = $state<HTMLDialogElement>();
	let activateMessage = $state<string | null>(null);

	function openArchive() {
		archiveMessage = null;
		archiveDialog?.showModal();
	}

	function openActivate() {
		activateMessage = null;
		activateDialog?.showModal();
	}

	const submitStatus: SubmitFunction = ({ formData }) => {
		pendingStatus = formData.get('status') as WorkspaceStatus;
		archiveMessage = null;
		activateMessage = null;
		return async ({ result }) => {
			if (result.type === 'success') {
				archiveDialog?.close();
				activateDialog?.close();
				await invalidateAll();
				showToast(t('ws.status.updated'), 'success');
			} else if (result.type === 'redirect') {
				await applyAction(result);
			} else if (result.type === 'failure') {
				const msg = (result.data?.message as string) ?? t('err.generic');
				if (pendingStatus === 'archive' && archiveDialog?.open) archiveMessage = msg;
				else if (pendingStatus === 'active' && activateDialog?.open) activateMessage = msg;
				else showToast(msg, 'error');
			} else {
				showToast(t('err.generic'), 'error');
			}
			pendingStatus = null;
		};
	};

	// --- Edit name & description ---
	let editDialog = $state<HTMLDialogElement>();
	let editAlertEl = $state<HTMLDivElement>();
	let editName = $state('');
	let editDescription = $state('');
	let editSubmitting = $state(false);
	let editMessage = $state<string | null>(null);
	let editFieldErrors = $state<Record<string, string>>({});

	function openEdit() {
		editName = ws.name;
		editDescription = ws.description ?? '';
		editMessage = null;
		editFieldErrors = {};
		editDialog?.showModal();
		tick().then(() => editDialog?.querySelector<HTMLInputElement>('#edit-name')?.focus());
	}

	function focusEditError() {
		if (editFieldErrors.name) editDialog?.querySelector<HTMLInputElement>('#edit-name')?.focus();
		else if (editMessage) editAlertEl?.focus();
	}

	const submitEdit: SubmitFunction = () => {
		editSubmitting = true;
		return async ({ result }) => {
			editSubmitting = false;
			if (result.type === 'success') {
				editDialog?.close();
				await invalidateAll();
				showToast(t('ws.edit.saved'), 'success');
			} else if (result.type === 'redirect') {
				editDialog?.close();
				await applyAction(result); // reslugged — lands on the new slug
			} else if (result.type === 'failure') {
				editMessage = (result.data?.message as string | null) ?? null;
				editFieldErrors = (result.data?.fieldErrors as Record<string, string>) ?? {};
				await tick();
				focusEditError();
			} else {
				editMessage = t('err.generic');
				await tick();
				focusEditError();
			}
		};
	};

	// --- Delete (type-to-confirm) ---
	let deleteDialog = $state<HTMLDialogElement>();
	let deleteConfirm = $state('');
	let deleteSubmitting = $state(false);
	let deleteMessage = $state<string | null>(null);
	const deleteReady = $derived(deleteConfirm.trim() === ws.name);

	function openDelete() {
		deleteConfirm = '';
		deleteMessage = null;
		deleteDialog?.showModal();
	}

	const submitDelete: SubmitFunction = ({ cancel }) => {
		if (!deleteReady) return cancel();
		deleteSubmitting = true;
		return async ({ result }) => {
			deleteSubmitting = false;
			if (result.type === 'redirect') {
				deleteDialog?.close();
				await applyAction(result); // → /workspace
			} else if (result.type === 'failure') {
				deleteMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				deleteMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head><title>{ws.name} · {t('brand.name')}</title></svelte:head>

{#snippet hero()}
	<header class="relative overflow-hidden rounded-box border border-base-content/10 bg-base-100">
		<svg
			aria-hidden="true"
			class="pointer-events-none absolute inset-0 h-full w-full"
			viewBox="0 0 640 200"
			preserveAspectRatio="xMinYMid slice"
			style="color: {heroColor}"
		>
			{#each [0, 1, 2, 3, 4, 5] as ring (ring)}
				<circle
					cx="72"
					cy="100"
					r={44 + heroPhase + ring * 46}
					fill="none"
					stroke="currentColor"
					stroke-width="1"
					opacity={0.15 - ring * 0.02}
				/>
			{/each}
		</svg>
		<div class="relative flex flex-wrap items-start gap-4 p-6 sm:p-8">
			<div
				class="flex h-16 w-16 flex-none items-center justify-center rounded-box text-xl font-semibold text-white"
				style="background: {heroColor}"
			>
				{monogram}
			</div>
			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
					<h1
						class="truncate text-2xl font-semibold tracking-[-0.02em] sm:text-3xl"
						title={ws.name}
					>
						{ws.name}
					</h1>
					<WorkspaceStatusBadge {status} />
				</div>
				{#if ws.description}
					<p class="mt-1.5 max-w-[65ch] text-sm leading-relaxed text-muted text-pretty">
						{ws.description}
					</p>
				{/if}
				<p class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-0.5 font-mono text-xs text-muted">
					<span class="whitespace-nowrap">{ws.slug} <span aria-hidden="true">·</span></span>
					<span class="whitespace-nowrap">
						{t('ws.detail.created')}
						{fmtDate(ws.created_at)} <span aria-hidden="true">·</span>
					</span>
					<span class="whitespace-nowrap">{t('ws.detail.updated')} {fmtDate(ws.updated_at)}</span>
				</p>
			</div>
			{#if canEdit}
				<Button variant="ghost" onclick={openEdit}>{t('ws.edit.open')}</Button>
			{/if}
		</div>
	</header>
{/snippet}

{#if !roomOpen}
	<div class="mx-auto w-full max-w-3xl px-6 py-8">
		{@render hero()}
		<div class="mt-6 rounded-box border border-base-content/10 bg-base-100 p-6">
			<h2 class="text-sm font-semibold">{t('room.notOpen.title')}</h2>
			<p class="mt-1.5 max-w-[52ch] text-sm text-muted text-pretty">{t('room.notOpen.body')}</p>
		</div>
	</div>
{:else}
	<div class="mx-auto w-full max-w-3xl px-6 py-8">
		{@render hero()}

		{#if managesRoom}
			<section class="mt-8">
				<h2 class="text-sm font-semibold">{t('ws.overview.summary')}</h2>
				<nav
					class="mt-3 flex flex-wrap items-center gap-x-6 gap-y-2"
					aria-label={t('ws.overview.summary')}
				>
					<a href={documentsHref} class="group flex items-baseline gap-2 text-sm">
						<span class="font-medium underline-offset-2 group-hover:underline">
							{t('ws.overview.quick.documents')}
						</span>
						{#if summary}
							<span class="font-mono text-xs text-muted">
								{t('ws.overview.count.documents', { n: summary.document_count })} · {t(
									'ws.overview.count.folders',
									{ n: summary.folder_count }
								)}
							</span>
						{/if}
					</a>
					<a href={membersHref} class="group flex items-baseline gap-2 text-sm">
						<span class="font-medium underline-offset-2 group-hover:underline">
							{t('ws.overview.quick.members')}
						</span>
						{#if summary}
							<span class="font-mono text-xs text-muted">
								{t('ws.overview.count.guests', { n: summary.guest_count })}
							</span>
						{/if}
					</a>
					<a
						href={inviteHref}
						class="btn btn-ghost btn-sm border border-base-content/10 font-normal sm:ms-auto"
					>
						<span class="font-medium">{t('ws.overview.quick.invite')}</span>
					</a>
				</nav>

				<div class="mt-6 flex items-baseline justify-between gap-3">
					<h3 class="text-xs font-medium tracking-wide text-muted">
						{t('ws.overview.recentActivity')}
					</h3>
					<a href={activityHref} class="text-xs underline-offset-2 hover:underline">
						{t('ws.overview.seeAll')}
					</a>
				</div>
				{#if recentActivity.length === 0}
					<p class="mt-2 text-sm text-muted">{t('ws.overview.recentActivity.empty')}</p>
				{:else}
					<ul class="mt-2 flex flex-col">
						{#each recentActivity as item (item.id)}
							{@const phrase = describeActivity(item)}
							<li
								class="flex items-baseline justify-between gap-3 border-b border-base-content/5 py-2 last:border-b-0"
							>
								<p class="min-w-0 flex-1 truncate text-sm">
									<span class="font-medium">
										{item.actor_name ||
											(item.actor_id ? item.actor_id.slice(0, 8) : t('activity.actor.system'))}
									</span>
									{#if phrase.key}
										{t(phrase.key, phrase.vars)}
									{:else}
										<code class="font-mono text-xs">{item.action}</code>
										{item.target_name}
									{/if}
								</p>
								<time
									datetime={item.created_at}
									class="flex-none font-mono text-xs whitespace-nowrap text-muted tabular-nums"
								>
									{fmtDate(item.created_at)}
								</time>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else}
			<section class="mt-8">
				<h2 class="text-sm font-semibold">{t('ws.overview.recents')}</h2>
				{#if recentFolders.length === 0 && recentDocs.length === 0}
					<p class="mt-2 max-w-[52ch] text-sm text-muted text-pretty">
						{t('ws.overview.recents.empty')}
					</p>
				{:else}
					<div class="mt-3 grid gap-x-8 gap-y-5 sm:grid-cols-2">
						<div>
							<h3 class="text-xs font-medium tracking-wide text-muted">
								{t('ws.overview.recents.folders')}
							</h3>
							{#if recentFolders.length === 0}
								<p class="mt-2 text-sm text-muted">{t('ws.overview.recents.empty')}</p>
							{:else}
								<ul class="mt-1 flex flex-col">
									{#each recentFolders as f (f.id)}
										<li>
											<a
												href={folderHref(f.id)}
												class="flex items-baseline justify-between gap-3 border-b border-base-content/5 py-2 text-sm underline-offset-2 last:border-b-0 hover:underline"
											>
												<span class="min-w-0 truncate">{f.name}</span>
												<span class="flex-none font-mono text-xs text-muted">{fmtStamp(f.at)}</span>
											</a>
										</li>
									{/each}
								</ul>
							{/if}
						</div>
						<div>
							<h3 class="text-xs font-medium tracking-wide text-muted">
								{t('ws.overview.recents.documents')}
							</h3>
							{#if recentDocs.length === 0}
								<p class="mt-2 text-sm text-muted">{t('ws.overview.recents.empty')}</p>
							{:else}
								<ul class="mt-1 flex flex-col">
									{#each recentDocs as d (d.id)}
										<li>
											<a
												href={docHref(d)}
												class="flex items-baseline justify-between gap-3 border-b border-base-content/5 py-2 text-sm underline-offset-2 last:border-b-0 hover:underline"
											>
												<span class="min-w-0 truncate">{d.name}</span>
												<span class="flex-none font-mono text-xs text-muted">{fmtStamp(d.at)}</span>
											</a>
										</li>
									{/each}
								</ul>
							{/if}
						</div>
					</div>
					<p class="mt-3 text-xs text-muted">{t('ws.overview.recents.device')}</p>
				{/if}
			</section>
		{/if}

		<!-- Status — the hero badge states it; the controls offer only legal transitions
	     (server map: prepare→active|archive, active→archive, archive→active). -->
		<section class="mt-10 border-t border-base-content/10 pt-6">
			{#if canEdit && status === 'prepare'}
				<h2 class="text-sm font-semibold">{t('ws.status.label')}</h2>
				<p class="mt-2 max-w-[52ch] text-sm text-muted text-pretty">{statusHint}</p>
				<div class="mt-5 rounded-box border border-base-content/10 bg-base-100 p-5">
					<h3 class="text-sm font-semibold">{t('room.activate.title')}</h3>
					<p class="mt-1 max-w-[52ch] text-sm text-muted text-pretty">
						{t('room.activate.body')}
					</p>
					<div class="mt-4 flex flex-wrap items-center gap-2">
						<Button onclick={openActivate}>{t('room.activate.submit')}</Button>
						{#if canTransitionRoom(status, 'archive')}
							<Button variant="ghost" onclick={openArchive}>{t('room.archive.open')}</Button>
						{/if}
					</div>
				</div>
			{:else}
				<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
					<div class="min-w-0">
						<h2 class="text-sm font-semibold">{t('ws.status.label')}</h2>
						<p class="mt-2 max-w-[52ch] text-sm text-muted text-pretty">{statusHint}</p>
					</div>
					{#if canEdit && status === 'active'}
						<div class="flex-none">
							<Button variant="ghost" onclick={openArchive}>{t('room.archive.open')}</Button>
						</div>
					{:else if canEdit && status === 'archive'}
						<form
							method="POST"
							action="?/updateStatus"
							use:enhance={submitStatus}
							class="flex-none"
						>
							<input type="hidden" name="status" value="active" />
							<Button type="submit" loading={pendingStatus === 'active'}>
								{t('room.unarchive.submit')}
							</Button>
						</form>
					{/if}
				</div>
			{/if}
		</section>

		<!-- Archive packages -->
		{#if managesRoom}
			<section class="mt-8 border-t border-base-content/10 pt-5">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div class="min-w-0">
						<h2 class="text-sm font-semibold">{t('archive.title')}</h2>
						<p class="mt-1 max-w-[52ch] text-sm text-muted text-pretty">{t('archive.body')}</p>
					</div>
					<form method="POST" action="?/createArchive" use:enhance={submitExport} class="flex-none">
						<Button
							type="submit"
							variant="ghost"
							loading={exportSubmitting}
							disabled={archivePending}
						>
							{t('archive.create')}
						</Button>
					</form>
				</div>

				{#if archives.length === 0}
					<p class="mt-4 text-sm text-muted">{t('archive.empty')}</p>
				{:else}
					<ul class="mt-4 flex flex-col gap-2">
						{#each archives as a (a.id)}
							<li
								class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-box border border-base-content/10 bg-base-100 px-4 py-3"
							>
								<div class="min-w-0">
									<p class="flex flex-wrap items-center gap-x-2 text-sm">
										<span class="font-medium">{t(`archive.status.${a.status}`)}</span>
										{#if a.status === 'ready'}
											<span class="font-mono text-xs text-muted">{formatBytes(a.size_bytes)}</span>
											<span aria-hidden="true" class="text-muted">·</span>
											<span class="font-mono text-xs text-muted">
												{t('archive.documents', { n: a.document_count })}
											</span>
											{#if a.missing_count > 0}
												<span aria-hidden="true" class="text-muted">·</span>
												<span class="font-mono text-xs text-muted">
													{t('archive.missing', { n: a.missing_count })}
												</span>
											{/if}
										{/if}
									</p>
									<p class="mt-0.5 font-mono text-xs text-muted">
										{fmtDate(a.created_at)} · {a.requested_by_name} · {t('archive.expires', {
											date: fmtDate(a.expires_at)
										})}
									</p>
									{#if a.status === 'failed' && a.error}
										<p class="mt-1 text-xs text-muted">{a.error}</p>
									{/if}
									{#if a.status === 'ready' && a.checksum_sha256}
										<p class="mt-1 truncate font-mono text-[0.6875rem] text-muted">
											sha256 {a.checksum_sha256}
										</p>
									{/if}
								</div>
								<div class="flex flex-none items-center gap-2">
									{#if a.status === 'ready'}
										<!-- Plain link: a room-sized ZIP must stream to disk, never through a Blob. -->
										<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- endpoint, not a page: resolve() has no entry for /api routes -->
										<a href={archiveHref(a.id)} class="btn btn-ghost btn-sm" download>
											{t('archive.download')}
										</a>
									{/if}
									{#if !readOnly && a.status !== 'pending'}
										<button
											type="button"
											class="btn btn-ghost btn-sm text-error"
											onclick={() => openArchiveDelete(a)}
										>
											{t('archive.delete')}
										</button>
									{/if}
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}

		<!-- Delete — quiet settings-style row; the type-to-confirm dialog carries the danger. -->
		{#if canDelete}
			<section
				class="mt-8 flex flex-col gap-3 border-t border-base-content/10 pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6"
			>
				<div class="min-w-0">
					<h2 class="text-sm font-semibold">{t('ws.delete.title')}</h2>
					<p class="mt-1 max-w-[48ch] text-sm text-muted text-pretty">
						{readOnly ? t('room.delete.blocked') : t('ws.delete.body')}
					</p>
				</div>
				<div class="flex-none">
					<Button variant="danger-outline" disabled={readOnly} onclick={openDelete}>
						{t('ws.delete.open')}
					</Button>
				</div>
			</section>
		{/if}
	</div>
{/if}

<!-- Edit dialog -->
<dialog bind:this={editDialog} class="modal" aria-labelledby="edit-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="edit-title" class="text-lg font-semibold tracking-[-0.01em]">{t('ws.edit.title')}</h2>

		{#if editMessage}
			<div bind:this={editAlertEl} tabindex="-1" class="mt-4 outline-none">
				<Alert align="start">{editMessage}</Alert>
			</div>
		{/if}

		<form method="POST" action="?/update" use:enhance={submitEdit} class="mt-4 flex flex-col gap-4">
			<Field
				id="edit-name"
				name="name"
				label={t('ws.field.name')}
				bind:value={editName}
				placeholder={t('ws.field.namePlaceholder')}
				required
				maxlength={120}
				error={editFieldErrors.name}
			/>
			<TextareaField
				id="edit-description"
				name="description"
				label={t('ws.field.description')}
				bind:value={editDescription}
				placeholder={t('ws.field.descriptionPlaceholder')}
				hint={t('ws.field.descriptionHint')}
				maxlength={500}
				error={editFieldErrors.description}
			/>
			<div class="mt-2 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => editDialog?.close()}>
					{t('ws.dialog.cancel')}
				</Button>
				<Button type="submit" loading={editSubmitting}>
					{editSubmitting ? t('ws.dialog.submitting') : t('ws.edit.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<!-- Delete dialog -->
<dialog bind:this={deleteDialog} class="modal" aria-labelledby="delete-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('ws.delete.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('ws.delete.warning', { name: ws.name })}</p>

		{#if deleteMessage}
			<div class="mt-4"><Alert align="start">{deleteMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/delete"
			use:enhance={submitDelete}
			class="mt-4 flex flex-col gap-4"
		>
			<Field
				id="delete-confirm"
				name="confirm"
				label={t('ws.delete.confirmLabel', { name: ws.name })}
				bind:value={deleteConfirm}
				placeholder={ws.name}
				autocomplete="off"
			/>
			<div class="mt-2 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => deleteDialog?.close()}>
					{t('ws.dialog.cancel')}
				</Button>
				<Button type="submit" variant="danger" disabled={!deleteReady} loading={deleteSubmitting}>
					{deleteSubmitting ? t('ws.delete.submitting') : t('ws.delete.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<!-- Archive confirmation — counted button, not type-to-confirm: archiving is
     reversible and must not read like deletion. -->
<dialog bind:this={archiveDialog} class="modal" aria-labelledby="archive-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="archive-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('room.archive.title')}
		</h2>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.archive.body')}</p>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.archive.caveat')}</p>
		{#if managesRoom}
			<p class="mt-2 text-sm text-muted text-pretty">{t('room.archive.exportHint')}</p>
		{/if}

		{#if archiveMessage}
			<div class="mt-4"><Alert align="start">{archiveMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/updateStatus"
			use:enhance={submitStatus}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="status" value="archive" />
			<Button type="button" variant="ghost" onclick={() => archiveDialog?.close()}>
				{t('ws.dialog.cancel')}
			</Button>
			<Button type="submit" wrap loading={pendingStatus === 'archive'}>
				{guestCount > 0
					? t('room.archive.submitCount', { n: guestCount })
					: t('room.archive.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<dialog bind:this={activateDialog} class="modal" aria-labelledby="activate-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="activate-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('room.activate.title')}
		</h2>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.activate.confirmBody')}</p>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.activate.warning')}</p>

		{#if activateMessage}
			<div class="mt-4"><Alert align="start">{activateMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/updateStatus"
			use:enhance={submitStatus}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="status" value="active" />
			<Button type="button" variant="ghost" onclick={() => activateDialog?.close()}>
				{t('ws.dialog.cancel')}
			</Button>
			<Button type="submit" wrap loading={pendingStatus === 'active'}>
				{guestCount > 0
					? t('room.activate.submitCount', { n: guestCount })
					: t('room.activate.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<dialog bind:this={archiveDeleteDialog} class="modal" aria-labelledby="archive-delete-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="archive-delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('archive.delete.title')}
		</h2>
		{#if archiveDeleteTarget}
			<p class="mt-2 text-sm text-muted text-pretty">
				{t('archive.delete.warning', { date: fmtDate(archiveDeleteTarget.created_at) })}
			</p>
		{/if}

		{#if archiveDeleteMessage}
			<div class="mt-4"><Alert align="start">{archiveDeleteMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/deleteArchive"
			use:enhance={submitArchiveDelete}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="archive_id" value={archiveDeleteTarget?.id ?? ''} />
			<Button type="button" variant="ghost" onclick={() => archiveDeleteDialog?.close()}>
				{t('ws.dialog.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={archiveDeleteSubmitting}>
				{t('archive.delete.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<Toaster />
