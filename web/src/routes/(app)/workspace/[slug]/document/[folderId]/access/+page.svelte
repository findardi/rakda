<script lang="ts">
	import { enhance } from '$app/forms';
	import { beforeNavigate, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import { findNode } from '$lib/tree';
	import type { DirectFolderAccess, InheritedFolderAccess } from '$lib/types/content';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);
	const folders = $derived(data.folders);
	const groups = $derived(data.groups);
	const ready = $derived(data.accessReady);
	const folder = $derived(findNode(folders, folderId));

	const targetId = $derived(navigating.to?.params?.folderId ?? folderId);
	const switching = $derived(targetId !== folderId);
	const shownFolder = $derived(switching ? findNode(folders, targetId) : folder);

	const SKELETON_ROWS = [42, 55, 33];

	const direct = $derived(data.panel.direct);
	const inherited = $derived(data.panel.inherited);
	const maxPages = $derived(data.watermarkMaxPages);

	const limitedByWatermark = (c: Caps) => c.can_download && c.can_watermark;

	type Caps = {
		can_view: boolean;
		can_download: boolean;
		can_watermark: boolean;
		can_download_original: boolean;
	};

	const BLOCKED: Caps = {
		can_view: false,
		can_download: false,
		can_watermark: false,
		can_download_original: false
	};
	const DEFAULT_CAPS: Caps = {
		can_view: true,
		can_download: false,
		can_watermark: false,
		can_download_original: false
	};

	function capsOf(row: Caps): Caps {
		return {
			can_view: row.can_view,
			can_download: row.can_download,
			can_watermark: row.can_watermark,
			can_download_original: row.can_download_original
		};
	}

	function capsEqual(a: Caps, b: Caps): boolean {
		return (
			a.can_view === b.can_view &&
			a.can_download === b.can_download &&
			a.can_watermark === b.can_watermark &&
			a.can_download_original === b.can_download_original
		);
	}

	function capsLabel(c: Caps): string {
		if (!c.can_view) return t('level.none');
		return c.can_download ? t('level.download') : t('level.view');
	}

	// Column order mirrors the flow a reader takes: see → marked → take → take clean.
	const CAP_KEYS = ['can_view', 'can_watermark', 'can_download', 'can_download_original'] as const;
	const COLS = CAP_KEYS.length + 2;

	function capLabel(key: keyof Caps): string {
		if (key === 'can_view') return t('facc.cap.view');
		if (key === 'can_watermark') return t('facc.cap.watermark');
		if (key === 'can_download') return t('facc.cap.download');
		return t('facc.cap.downloadOriginal');
	}

	function capHint(key: keyof Caps): string {
		if (key === 'can_view') return t('facc.cap.viewHint');
		if (key === 'can_watermark') return t('facc.cap.watermarkHint');
		if (key === 'can_download') return t('facc.cap.downloadHint');
		return t('facc.cap.downloadOriginalHint');
	}

	function toggleCap(base: Caps, key: keyof Caps, value: boolean): Caps {
		const next: Caps = { ...base, [key]: value };
		if (key === 'can_view' && !value) {
			next.can_download = false;
			next.can_watermark = false;
			next.can_download_original = false;
		}
		if ((key === 'can_download' || key === 'can_watermark') && value) {
			next.can_view = true;
		}
		if (key === 'can_download' && !value) {
			next.can_download_original = false;
		}
		if (key === 'can_download_original' && value) {
			next.can_download = true;
			next.can_view = true;
			next.can_watermark = false;
		}
		if (key === 'can_watermark' && value) {
			next.can_download_original = false;
		}
		return next;
	}

	function focusHere(node: HTMLElement) {
		node.focus();
	}

	let formError = $state<string | null>(null);
	let errorScope = $state<string | null>(null);
	let status = $state<string | null>(null);

	let staged = $state<Record<string, Caps>>({});
	let confirmRevoke = $state<string | null>(null);
	let confirmBlock = $state<string | null>(null);

	let adding = $state(false);
	let addGroupId = $state('');
	let addCaps = $state<Caps>({ ...DEFAULT_CAPS });
	let addSubmitting = $state(false);
	let saving = $state(false);
	let revokingGroup = $state<string | null>(null);
	let blockingGroup = $state<string | null>(null);

	const directIds = $derived(new Set(direct.map((r) => r.group_id)));
	const addable = $derived(groups.filter((g) => !directIds.has(g.id)));

	const inheritedOf = $derived(new Map(inherited.map((r) => [r.group_id, r])));

	const descendants = $derived.by(() => {
		const count = (nodes: typeof folders): number =>
			nodes.reduce((n, c) => n + 1 + count(c.children ?? []), 0);
		return folder ? count(folder.children ?? []) : 0;
	});

	const stagedOf = (row: DirectFolderAccess): Caps => staged[row.group_id] ?? capsOf(row);
	const isDirty = (row: DirectFolderAccess): boolean => {
		const s = staged[row.group_id];
		return !!s && !capsEqual(s, capsOf(row));
	};

	const dirtyRows = $derived(direct.filter((r) => isDirty(r)));
	const hasUnsaved = $derived(adding || dirtyRows.length > 0);

	// Column header = the whole column: pressed when every group has it,
	// ink (no fill) when some do, muted when none.
	const allOn = (key: keyof Caps) => direct.length > 0 && direct.every((r) => stagedOf(r)[key]);
	const someOn = (key: keyof Caps) => direct.some((r) => stagedOf(r)[key]);

	function toggleColumn(key: keyof Caps) {
		const value = !allOn(key);
		for (const row of direct) staged[row.group_id] = toggleCap(stagedOf(row), key, value);
	}

	// Persisted rows only: while editing, the footer's consequence line already says it.
	const anyLimited = $derived(direct.some((r) => limitedByWatermark(capsOf(r))));

	let settledFor = $state('');

	$effect(() => {
		if (settledFor === folderId) return;
		settledFor = folderId;
		staged = {};
		confirmRevoke = null;
		confirmBlock = null;
		adding = false;
		addGroupId = '';
		addCaps = { ...DEFAULT_CAPS };
		formError = null;
		errorScope = null;
		status = null;
	});

	beforeNavigate((nav) => {
		if (hasUnsaved && !confirm(t('facc.leave.warn'))) nav.cancel();
	});

	function raises(from: Caps | null, to: Caps): boolean {
		if (!from) return to.can_view || to.can_download || to.can_download_original;
		return (
			(to.can_view && !from.can_view) ||
			(to.can_download && !from.can_download) ||
			(to.can_download_original && !from.can_download_original)
		);
	}

	const escalatingAny = $derived(dirtyRows.some((r) => raises(capsOf(r), stagedOf(r))));

	function consequence(group: string, caps: Caps): string {
		const n = descendants;
		let base: string;
		if (!caps.can_view) {
			base = n ? t('facc.will.blockSub', { group, n }) : t('facc.will.block', { group });
		} else if (caps.can_download) {
			base = n ? t('facc.will.downloadSub', { group, n }) : t('facc.will.download', { group });
		} else {
			base = n ? t('facc.will.viewSub', { group, n }) : t('facc.will.view', { group });
		}
		if (caps.can_view && caps.can_watermark) base += ' ' + t('facc.will.wmOn', { group });
		if (limitedByWatermark(caps) && maxPages !== null) {
			base += ' ' + t('facc.will.wmLimit', { max: maxPages });
		}
		if (caps.can_download_original) base += ' ' + t('facc.will.origOn', { group });
		return base;
	}

	function setRowCap(row: DirectFolderAccess, key: keyof Caps, value: boolean) {
		staged[row.group_id] = toggleCap(stagedOf(row), key, value);
	}

	function setAddCap(key: keyof Caps, value: boolean) {
		addCaps = toggleCap(addCaps, key, value);
	}

	// One action, one shape: every save posts `rows` — the footer posts every
	// dirty group, add and block post one.
	const rowsOf = (rows: { group_id: string; caps: Caps }[]) =>
		JSON.stringify(rows.map(({ group_id, caps }) => ({ group_id, ...caps })));

	const savePayload = $derived(
		rowsOf(dirtyRows.map((r) => ({ group_id: r.group_id, caps: stagedOf(r) })))
	);
	const addPayload = $derived(rowsOf([{ group_id: addGroupId, caps: addCaps }]));

	const addInherits = $derived(addGroupId ? (inheritedOf.get(addGroupId) ?? null) : null);
	const addGroupName = $derived(groups.find((g) => g.id === addGroupId)?.name ?? '');

	function startAdd() {
		confirmRevoke = null;
		confirmBlock = null;
		adding = true;
		addGroupId = addable[0]?.id ?? '';
		addCaps = { ...DEFAULT_CAPS };
		formError = null;
		errorScope = null;
	}

	function startOverride(row: InheritedFolderAccess) {
		confirmRevoke = null;
		confirmBlock = null;
		adding = true;
		addGroupId = row.group_id;
		addCaps = capsOf(row);
		formError = null;
		errorScope = null;
	}

	function openRevoke(row: DirectFolderAccess) {
		const next = confirmRevoke === row.group_id ? null : row.group_id;
		confirmRevoke = next;
		if (next) {
			confirmBlock = null;
			delete staged[row.group_id];
		}
	}

	function failureOf(result: { type: string; data?: Record<string, unknown> }): string {
		return result.type === 'failure'
			? ((result.data?.message as string) ?? t('err.generic'))
			: t('err.generic');
	}

	const submitSave: SubmitFunction = () => {
		saving = true;
		formError = null;
		errorScope = null;
		const rows = dirtyRows;
		const only = rows.length === 1 ? rows[0]! : null;
		const onlyBlocked = only ? !stagedOf(only).can_view : false;
		return async ({ result }) => {
			saving = false;
			if (result.type === 'success') {
				staged = {};
				await invalidateAll();
				status = only
					? onlyBlocked
						? t('facc.blockedNow', { group: only.group_name })
						: t('facc.saved', { group: only.group_name })
					: t('facc.savedN', { n: rows.length });
			} else {
				// A partial failure keeps what landed; only the rest stays staged.
				const saved =
					result.type === 'failure' ? ((result.data?.saved as string[] | undefined) ?? []) : [];
				for (const id of saved) delete staged[id];
				if (saved.length) await invalidateAll();
				formError = failureOf(result);
				errorScope = 'save';
			}
		};
	};

	const submitBlock =
		(row: InheritedFolderAccess): SubmitFunction =>
		() => {
			blockingGroup = row.group_id;
			formError = null;
			errorScope = null;
			return async ({ result }) => {
				blockingGroup = null;
				if (result.type === 'success') {
					confirmBlock = null;
					await invalidateAll();
					status = t('facc.blockedNow', { group: row.group_name });
				} else {
					formError = failureOf(result);
					errorScope = row.group_id;
				}
			};
		};

	const submitRevoke =
		(row: DirectFolderAccess): SubmitFunction =>
		() => {
			revokingGroup = row.group_id;
			formError = null;
			errorScope = null;
			const back = row.shadows;
			return async ({ result }) => {
				revokingGroup = null;
				if (result.type === 'success') {
					confirmRevoke = null;
					await invalidateAll();
					status = back
						? t('facc.revokedInherits', { group: row.group_name, name: back.source_folder_name })
						: t('facc.revoked', { group: row.group_name });
				} else {
					formError = failureOf(result);
					errorScope = row.group_id;
				}
			};
		};

	const submitAdd: SubmitFunction = ({ cancel }) => {
		if (!addGroupId) {
			formError = t('facc.err.pick');
			errorScope = 'add';
			cancel();
			return;
		}
		const groupName = addGroupName;
		const blocked = !addCaps.can_view;
		addSubmitting = true;
		formError = null;
		errorScope = null;
		return async ({ result }) => {
			addSubmitting = false;
			if (result.type === 'success') {
				adding = false;
				addGroupId = '';
				await invalidateAll();
				status = blocked
					? t('facc.blockedNow', { group: groupName })
					: t('facc.saved', { group: groupName });
			} else {
				formError = failureOf(result);
				errorScope = 'add';
			}
		};
	};
</script>

{#snippet lockIcon()}
	<svg
		class="h-3.5 w-3.5 flex-none text-error"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<rect x="4" y="10.5" width="16" height="10" rx="2" />
		<path d="M8 10.5V7a4 4 0 0 1 8 0v3.5" />
	</svg>
{/snippet}

{#snippet capIcon(key: keyof Caps)}
	<svg
		class="h-4 w-4"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if key === 'can_view'}
			<path d="M3 12s3.5-7 9-7 9 7 9 7-3.5 7-9 7-9-7-9-7z" />
			<circle cx="12" cy="12" r="2.5" />
		{:else if key === 'can_download'}
			<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5" />
			<path d="M5 19h14" />
		{:else}
			<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
			<path d="M14 3v5h5" />
			{#if key === 'can_watermark'}
				<circle cx="12" cy="14.5" r="2" fill="currentColor" stroke="none" />
			{/if}
		{/if}
	</svg>
{/snippet}

{#snippet actionIcon(kind: 'revoke' | 'block' | 'override')}
	<svg
		class="h-4 w-4"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if kind === 'revoke'}
			<path d="M18 6 6 18M6 6l12 12" />
		{:else if kind === 'block'}
			<circle cx="12" cy="12" r="9" />
			<path d="m5.6 5.6 12.8 12.8" />
		{:else}
			<path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4z" />
		{/if}
	</svg>
{/snippet}

{#snippet capCell(
	key: keyof Caps,
	group: string,
	checked: boolean,
	disabled: boolean,
	onChange?: (v: boolean) => void
)}
	<td class="py-1.5 text-center">
		<input
			type="checkbox"
			class="checkbox checkbox-sm align-middle"
			{checked}
			{disabled}
			aria-label="{capLabel(key)} — {group}"
			onchange={(e) => onChange?.(e.currentTarget.checked)}
		/>
	</td>
{/snippet}

{#snippet consequenceBox(lines: string[], escalating: boolean, scope: string)}
	<div
		class="rounded-field border p-2.5 {escalating
			? 'border-warning/50 bg-warning/8'
			: 'border-base-content/10'}"
	>
		<ul class="space-y-1 text-xs" aria-live="polite">
			{#each lines as line, i (i)}
				<li class="text-pretty">{line}</li>
			{/each}
		</ul>
		<p class="mt-1 text-[0.6875rem] text-muted text-pretty">
			{t('facc.cap.rule')}
			{t('facc.cap.exclusive')}
		</p>
		{#if formError && errorScope === scope}
			<div class="mt-2"><Alert align="start">{formError}</Alert></div>
		{/if}
	</div>
{/snippet}

<section class="min-h-64 rounded-box border border-base-content/10 bg-base-100">
	<header
		class="flex flex-wrap items-center justify-between gap-3 border-b border-base-content/8 px-4 py-2.5"
	>
		<div class="flex min-w-0 items-baseline gap-2">
			{#if shownFolder}
				<span class="font-mono text-xs tabular-nums text-muted">{shownFolder.number}</span>
			{/if}
			<h2
				class="min-w-0 truncate text-[0.9375rem] font-semibold tracking-[-0.01em]"
				title={shownFolder?.name}
			>
				{shownFolder?.name ?? t('doc.docs.unknownFolder')}
			</h2>
			{#if direct.length && !switching}
				<span class="flex-none font-mono text-xs text-muted">
					{t('facc.directCount', { n: direct.length })}
				</span>
			{/if}
		</div>
	</header>

	<div class="p-3">
		{#if switching}
			<div class="space-y-2" aria-busy="true">
				{#each SKELETON_ROWS as width (width)}
					<div class="flex items-center gap-2 py-2">
						<span class="rakda-skeleton h-3.5 rounded-selector" style="width: {width}%"></span>
						<span class="flex-1"></span>
						<span class="rakda-skeleton h-8 w-44 flex-none rounded-field"></span>
					</div>
				{/each}
			</div>
		{:else if !ready}
			<Alert align="start">{t('facc.err.load')}</Alert>
		{:else if groups.length === 0}
			<div class="rounded-box border border-base-content/10 p-5 text-center">
				<p class="text-sm font-semibold">{t('facc.noGroups.title')}</p>
				<p class="mx-auto mt-1 max-w-[46ch] text-sm text-muted text-pretty">
					{t('facc.noGroups.body')}
				</p>
				<a
					href={resolve('/(app)/workspace/[slug]/management-access/group', { slug })}
					class="mt-3 inline-block text-sm font-medium text-primary hover:underline"
				>
					{t('facc.noGroups.cta')}
				</a>
			</div>
		{:else}
			<p class="mb-2 flex items-start gap-2 text-xs text-muted text-pretty">
				<svg
					class="mt-px h-4 w-4 flex-none"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<circle cx="12" cy="12" r="9" />
					<path d="M12 16v-5M12 8h.01" />
				</svg>
				<span>{t('facc.flow')}</span>
			</p>

			{#if direct.length || inherited.length || adding}
				<div class="overflow-x-auto">
					<table class="w-full table-fixed border-collapse text-sm">
						<colgroup>
							<col />
							{#each CAP_KEYS as key (key)}
								<col class="w-11" />
							{/each}
							<col class="w-[4.5rem]" />
						</colgroup>
						<thead>
							<tr class="border-b border-base-content/10">
								<th scope="col" class="py-1 pr-2 text-left text-xs font-medium text-muted">
									{t('facc.add.group')}
								</th>
								{#each CAP_KEYS as key (key)}
									<th scope="col" class="py-1 text-center">
										<button
											type="button"
											class="rakda-cap inline-grid h-8 w-9 place-items-center rounded-[4px] align-middle transition-colors"
											class:is-some={!allOn(key) && someOn(key)}
											aria-pressed={allOn(key)}
											aria-label={t('facc.col.all', { cap: capLabel(key) })}
											title={capHint(key)}
											disabled={saving || direct.length === 0}
											onclick={() => toggleColumn(key)}
										>
											{@render capIcon(key)}
										</button>
									</th>
								{/each}
								<th scope="col"></th>
							</tr>
						</thead>

						<tbody>
							{#each direct as row (row.group_id)}
								{@const current = stagedOf(row)}
								{@const blocked = !row.can_view}
								{@const revoking = confirmRevoke === row.group_id}
								<tr
									class="border-base-content/8 {revoking ? '' : 'border-b'}"
									class:opacity-60={revoking}
								>
									<th scope="row" class="py-1.5 pr-2 text-left font-medium">
										<span class="flex min-w-0 items-center gap-1.5">
											{#if blocked}{@render lockIcon()}{/if}
											<span class="truncate {blocked ? 'text-error' : ''}" title={row.group_name}>
												{row.group_name}
											</span>
										</span>
									</th>
									{#each CAP_KEYS as key (key)}
										{@render capCell(key, row.group_name, current[key], saving || revoking, (v) =>
											setRowCap(row, key, v)
										)}
									{/each}
									<td class="py-1 text-right">
										<button
											type="button"
											onclick={() => openRevoke(row)}
											disabled={revokingGroup === row.group_id}
											aria-expanded={revoking}
											aria-label={t('facc.revokeOf', { group: row.group_name })}
											title={t('facc.revokeOf', { group: row.group_name })}
											class="inline-grid h-8 w-8 place-items-center rounded-field align-middle text-muted transition-colors hover:bg-error/10 hover:text-error disabled:pointer-events-none disabled:opacity-50"
										>
											{@render actionIcon('revoke')}
										</button>
									</td>
								</tr>

								{#if revoking}
									<tr class="border-b border-base-content/8">
										<td colspan={COLS} class="pb-2">
											<form
												method="POST"
												action="?/removeAccess"
												use:enhance={submitRevoke(row)}
												class="rounded-field border border-base-content/10 p-2.5"
											>
												<input type="hidden" name="groupId" value={row.group_id} />
												<p class="text-xs text-pretty" tabindex="-1" use:focusHere>
													{#if row.shadows}
														{t('facc.revoke.back', {
															group: row.group_name,
															level: capsLabel(row.shadows),
															name: row.shadows.source_folder_name
														})}
													{:else}
														{t('facc.revoke.gone', { group: row.group_name })}
													{/if}
												</p>
												{#if formError && errorScope === row.group_id}
													<div class="mt-2"><Alert align="start">{formError}</Alert></div>
												{/if}
												<div class="mt-2 flex justify-end gap-2">
													<Button
														type="button"
														variant="ghost"
														size="sm"
														onclick={() => (confirmRevoke = null)}
													>
														{t('facc.cancel')}
													</Button>
													<Button
														type="submit"
														variant="danger"
														size="sm"
														loading={revokingGroup === row.group_id}
													>
														{t('facc.revoke')}
													</Button>
												</div>
											</form>
										</td>
									</tr>
								{/if}
							{/each}

							{#if adding}
								<tr class="bg-base-200/40">
									<td class="py-1.5 pr-2">
										<select
											id="facc-add-group"
											name="groupId"
											form="facc-add"
											bind:value={addGroupId}
											use:focusHere
											aria-label={t('facc.add.group')}
											class="select select-sm w-full"
										>
											{#each addable as g (g.id)}
												{@const from = inheritedOf.get(g.id)}
												<option value={g.id}>
													{from
														? t('facc.add.inherits', {
																group: g.name,
																level: capsLabel(from),
																name: from.source_folder_name
															})
														: g.name}
												</option>
											{/each}
										</select>
									</td>
									{#each CAP_KEYS as key (key)}
										{@render capCell(key, addGroupName, addCaps[key], addSubmitting, (v) =>
											setAddCap(key, v)
										)}
									{/each}
									<td></td>
								</tr>
								<tr class="border-b border-base-content/8">
									<td colspan={COLS} class="pb-2">
										{#if addGroupId}
											{@render consequenceBox(
												[consequence(addGroupName, addCaps)],
												raises(addInherits ? capsOf(addInherits) : null, addCaps),
												'add'
											)}
										{:else if formError && errorScope === 'add'}
											<Alert align="start">{formError}</Alert>
										{/if}
										<div class="mt-2 flex justify-end gap-2">
											<Button
												type="button"
												variant="ghost"
												size="sm"
												onclick={() => (adding = false)}
											>
												{t('facc.add.cancel')}
											</Button>
											<Button
												type="submit"
												form="facc-add"
												size="sm"
												loading={addSubmitting}
												disabled={!addGroupId}
											>
												{addInherits ? t('facc.add.change') : t('facc.add.submit')}
											</Button>
										</div>
									</td>
								</tr>
							{/if}

							{#if inherited.length}
								<tr>
									<th
										scope="colgroup"
										colspan={COLS}
										class="pt-3 pb-1 text-left text-xs font-semibold text-muted"
									>
										{t('facc.inherited')}
										<span class="ml-1 font-mono font-normal">
											{t('facc.inheritedCount', { n: inherited.length })}
										</span>
									</th>
								</tr>
								{#each inherited as row (row.group_id)}
									{@const blocking = confirmBlock === row.group_id}
									<tr
										class="border-base-content/8 {blocking ? '' : 'border-b'}"
										class:opacity-60={blocking}
									>
										<th scope="row" class="py-1.5 pr-2 text-left font-medium">
											<span class="block truncate" title={row.group_name}>{row.group_name}</span>
											<span class="block truncate text-xs font-normal text-muted">
												{t('facc.inheritedFrom', { name: row.source_folder_name })}
											</span>
										</th>
										{#each CAP_KEYS as key (key)}
											{@render capCell(key, row.group_name, row[key], true)}
										{/each}
										<td class="py-1 text-right whitespace-nowrap">
											<button
												type="button"
												onclick={() => startOverride(row)}
												aria-label={t('facc.overrideOf', { group: row.group_name })}
												title={t('facc.override')}
												class="inline-grid h-8 w-8 place-items-center rounded-field align-middle text-muted transition-colors hover:bg-primary/8 hover:text-primary"
											>
												{@render actionIcon('override')}
											</button>
											<button
												type="button"
												onclick={() =>
													(confirmBlock = confirmBlock === row.group_id ? null : row.group_id)}
												aria-expanded={blocking}
												aria-label={t('facc.blockOf', { group: row.group_name })}
												title={t('facc.block')}
												class="inline-grid h-8 w-8 place-items-center rounded-field align-middle text-muted transition-colors hover:bg-error/10 hover:text-error"
											>
												{@render actionIcon('block')}
											</button>
										</td>
									</tr>

									{#if blocking}
										<tr class="border-b border-base-content/8">
											<td colspan={COLS} class="pb-2">
												<form
													method="POST"
													action="?/setAccess"
													use:enhance={submitBlock(row)}
													class="rounded-field border border-base-content/10 p-2.5"
												>
													<input
														type="hidden"
														name="rows"
														value={rowsOf([{ group_id: row.group_id, caps: BLOCKED }])}
													/>
													<p class="text-xs text-pretty" tabindex="-1" use:focusHere>
														{consequence(row.group_name, BLOCKED)}
													</p>
													{#if formError && errorScope === row.group_id}
														<div class="mt-2"><Alert align="start">{formError}</Alert></div>
													{/if}
													<div class="mt-2 flex justify-end gap-2">
														<Button
															type="button"
															variant="ghost"
															size="sm"
															onclick={() => (confirmBlock = null)}
														>
															{t('facc.cancel')}
														</Button>
														<Button
															type="submit"
															variant="danger"
															size="sm"
															loading={blockingGroup === row.group_id}
														>
															{t('facc.block')}
														</Button>
													</div>
												</form>
											</td>
										</tr>
									{/if}
								{/each}
							{/if}
						</tbody>
					</table>
				</div>

				<form id="facc-add" method="POST" action="?/setAccess" use:enhance={submitAdd}>
					<input type="hidden" name="rows" value={addPayload} />
				</form>

				{#if dirtyRows.length}
					<form method="POST" action="?/setAccess" use:enhance={submitSave} class="mt-3">
						<input type="hidden" name="rows" value={savePayload} />
						{@render consequenceBox(
							dirtyRows.map((r) => consequence(r.group_name, stagedOf(r))),
							escalatingAny,
							'save'
						)}
						<div class="mt-2 flex justify-end gap-2">
							<Button type="button" variant="ghost" size="sm" onclick={() => (staged = {})}>
								{t('facc.cancel')}
							</Button>
							<Button type="submit" size="sm" variant="primary" loading={saving}>
								{escalatingAny ? t('facc.escalate') : t('facc.save')}
							</Button>
						</div>
					</form>
				{/if}

				{#if anyLimited}
					<p class="mt-2 max-w-prose text-xs text-warning-ink text-pretty" role="status">
						{maxPages !== null
							? t('facc.cap.wmLimit', { max: maxPages })
							: t('facc.cap.wmLimitUnknown')}
					</p>
				{/if}
			{:else}
				<div class="rounded-box border border-base-content/10 p-5 text-center">
					<p class="text-sm font-semibold">{t('facc.empty.title')}</p>
					<p class="mx-auto mt-1 max-w-[48ch] text-sm text-muted text-pretty">
						{t('facc.empty.body')}
					</p>
				</div>
			{/if}

			{#if !adding}
				{#if addable.length}
					<button
						type="button"
						onclick={startAdd}
						class="mt-2 inline-flex items-center gap-1.5 rounded-field px-2 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
					>
						<svg
							class="h-4 w-4"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.8"
							stroke-linecap="round"
							aria-hidden="true"
						>
							<path d="M12 5v14M5 12h14" />
						</svg>
						{t('facc.add')}
					</button>
				{:else}
					<p class="mt-2 text-xs text-muted">{t('facc.allGranted')}</p>
				{/if}
			{/if}
		{/if}

		<p aria-live="polite" class="mt-2 text-xs text-muted text-pretty">{status ?? ''}</p>
	</div>
</section>

<style>
	/* Column header: fill = every group has it, ink = some, muted = none. */
	.rakda-cap[aria-pressed='true'] {
		background-color: var(--color-base-content);
		color: var(--color-base-100);
	}
	.rakda-cap[aria-pressed='false'] {
		color: var(--color-muted);
	}
	.rakda-cap[aria-pressed='false'].is-some {
		color: var(--color-base-content);
	}
	.rakda-cap[aria-pressed='false']:hover:not(:disabled) {
		background-color: color-mix(in oklch, var(--color-base-content) 6%, transparent);
		color: var(--color-base-content);
	}
	.rakda-cap:disabled {
		opacity: 0.5;
	}

	.rakda-skeleton {
		background-color: color-mix(in oklch, var(--color-base-content) 8%, transparent);
		animation: rakda-pulse 1400ms ease-in-out infinite;
	}
	@keyframes rakda-pulse {
		50% {
			opacity: 0.45;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-skeleton {
			animation: none;
		}
	}
</style>
