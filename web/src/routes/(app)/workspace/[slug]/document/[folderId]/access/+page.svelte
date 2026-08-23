<script lang="ts">
	import { enhance } from '$app/forms';
	import { beforeNavigate, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { cubicOut } from 'svelte/easing';
	import { prefersReducedMotion } from 'svelte/motion';
	import { slide } from 'svelte/transition';
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
	let savingGroup = $state<string | null>(null);
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

	const dirtyCount = $derived(direct.filter((r) => isDirty(r)).length);
	const hasUnsaved = $derived(adding || dirtyCount > 0);

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

	const addInherits = $derived(addGroupId ? (inheritedOf.get(addGroupId) ?? null) : null);

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

	const submitLevel =
		(row: DirectFolderAccess): SubmitFunction =>
		() => {
			savingGroup = row.group_id;
			formError = null;
			errorScope = null;
			const blocked = !stagedOf(row).can_view;
			return async ({ result }) => {
				savingGroup = null;
				if (result.type === 'success') {
					delete staged[row.group_id];
					await invalidateAll();
					status = blocked
						? t('facc.blockedNow', { group: row.group_name })
						: t('facc.saved', { group: row.group_name });
				} else {
					formError = failureOf(result);
					errorScope = row.group_id;
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
		const groupName = groups.find((g) => g.id === addGroupId)?.name ?? '';
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

{#snippet capCheck(
	label: string,
	checked: boolean,
	disabled: boolean,
	onChange: (v: boolean) => void,
	hint?: string
)}
	<label class="flex cursor-pointer items-center gap-2" title={hint}>
		<input
			type="checkbox"
			class="checkbox checkbox-sm"
			{checked}
			{disabled}
			onchange={(e) => onChange(e.currentTarget.checked)}
		/>
		<span class="text-xs font-medium">{label}</span>
	</label>
{/snippet}

{#snippet capToggles(
	caps: Caps,
	onChange: (key: keyof Caps, value: boolean) => void,
	disabled: boolean
)}
	<div class="flex flex-wrap gap-x-8 gap-y-3">
		<div>
			<span class="text-xs font-medium text-muted">{t('facc.cap.accessLegend')}</span>
			<div class="mt-1.5 flex flex-wrap items-center gap-x-5 gap-y-2">
				{@render capCheck(t('facc.cap.view'), caps.can_view, disabled, (v) =>
					onChange('can_view', v)
				)}
				{@render capCheck(t('facc.cap.download'), caps.can_download, disabled, (v) =>
					onChange('can_download', v)
				)}
				{@render capCheck(
					t('facc.cap.downloadOriginal'),
					caps.can_download_original,
					disabled,
					(v) => onChange('can_download_original', v),
					t('facc.cap.downloadOriginalHint')
				)}
			</div>
		</div>
		<div>
			<span class="text-xs font-medium text-muted">{t('facc.cap.protectLegend')}</span>
			<div class="mt-1.5 flex flex-wrap items-center gap-x-5 gap-y-2">
				{@render capCheck(
					t('facc.cap.watermark'),
					caps.can_watermark,
					disabled,
					(v) => onChange('can_watermark', v),
					t('facc.cap.watermarkHint')
				)}
			</div>
		</div>
	</div>
	<p class="mt-2 text-[0.6875rem] text-muted">{t('facc.cap.exclusive')}</p>
	{#if limitedByWatermark(caps)}
		<p
			role="status"
			transition:slide={{ duration: prefersReducedMotion.current ? 0 : 180, easing: cubicOut }}
			class="mt-2 flex max-w-prose items-start gap-1.5 text-xs text-warning-ink text-pretty"
		>
			<svg
				class="mt-0.5 h-3.5 w-3.5 flex-none"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<path d="M12 3 2.5 19.5h19z" />
				<path d="M12 9.5v4.5M12 16.5h.01" />
			</svg>
			<span>
				{maxPages !== null
					? t('facc.cap.wmLimit', { max: maxPages })
					: t('facc.cap.wmLimitUnknown')}
			</span>
		</p>
	{/if}
{/snippet}

{#snippet wmMark(limited: boolean)}
	<span
		class="flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
		title={limited && maxPages !== null
			? `${t('facc.cap.watermarkHint')} ${t('facc.cap.wmLimitChip', { max: maxPages })}`
			: t('facc.cap.watermarkHint')}
	>
		{t('facc.cap.watermark')}
	</span>
{/snippet}

{#snippet origMark()}
	<span
		class="flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
		title={t('facc.cap.downloadOriginalHint')}
	>
		{t('facc.cap.downloadOriginal')}
	</span>
{/snippet}

<section class="min-h-64 rounded-box border border-base-content/10 bg-base-100">
	<header
		class="flex flex-wrap items-center justify-between gap-3 border-b border-base-content/8 px-4 py-3"
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

	<div class="p-4">
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
			<p class="mb-4 flex items-start gap-2 text-xs text-muted text-pretty">
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

			<section>
				<h3 class="text-sm font-semibold">{t('facc.direct')}</h3>

				{#if direct.length}
					<ul class="mt-1 divide-y divide-base-content/8">
						{#each direct as row (row.group_id)}
							{@const current = stagedOf(row)}
							{@const blocked = !row.can_view}
							{@const dirty = isDirty(row)}
							{@const revoking = confirmRevoke === row.group_id}
							{@const escalating = raises(capsOf(row), current)}
							<li class="py-3">
								<div class="flex items-center gap-2">
									{#if blocked}{@render lockIcon()}{/if}

									<span
										title={row.group_name}
										class="min-w-0 flex-1 truncate text-sm font-medium {blocked
											? 'text-error'
											: ''}"
									>
										{row.group_name}
									</span>

									<button
										type="button"
										onclick={() => openRevoke(row)}
										disabled={revokingGroup === row.group_id}
										aria-expanded={revoking}
										aria-label={t('facc.revokeOf', { group: row.group_name })}
										title={t('facc.revoke')}
										class="grid h-9 w-9 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-error/10 hover:text-error disabled:pointer-events-none disabled:opacity-50"
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
											<path d="M18 6 6 18M6 6l12 12" />
										</svg>
									</button>
								</div>

								{#if !revoking}
									<div class="mt-2 pl-1">
										{@render capToggles(
											current,
											(k, v) => setRowCap(row, k, v),
											savingGroup === row.group_id
										)}
									</div>
								{/if}

								{#if dirty && !revoking}
									<form
										method="POST"
										action="?/setAccess"
										use:enhance={submitLevel(row)}
										class="mt-2 rounded-field border p-2.5
											{escalating ? 'border-warning/50 bg-warning/8' : 'border-base-content/10'}"
									>
										<input type="hidden" name="groupId" value={row.group_id} />
										<input type="hidden" name="canView" value={String(current.can_view)} />
										<input type="hidden" name="canDownload" value={String(current.can_download)} />
										<input
											type="hidden"
											name="canWatermark"
											value={String(current.can_watermark)}
										/>
										<input
											type="hidden"
											name="canDownloadOriginal"
											value={String(current.can_download_original)}
										/>
										<p class="text-xs text-pretty" aria-live="polite">
											{consequence(row.group_name, current)}
										</p>
										<p class="mt-1 text-[0.6875rem] text-muted">{t('facc.cap.rule')}</p>
										<div class="mt-2 flex justify-end gap-2">
											<Button
												type="button"
												variant="ghost"
												size="sm"
												onclick={() => delete staged[row.group_id]}
											>
												{t('facc.cancel')}
											</Button>
											<Button
												type="submit"
												size="sm"
												variant="primary"
												loading={savingGroup === row.group_id}
											>
												{escalating ? t('facc.escalate') : t('facc.save')}
											</Button>
										</div>
									</form>
								{/if}

								{#if revoking}
									<form
										method="POST"
										action="?/removeAccess"
										use:enhance={submitRevoke(row)}
										class="mt-2 rounded-field border border-base-content/10 p-2.5"
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
								{/if}

								{#if formError && errorScope === row.group_id}
									<div class="mt-2"><Alert align="start">{formError}</Alert></div>
								{/if}
							</li>
						{/each}
					</ul>
				{:else if inherited.length}
					<p class="mt-2 text-sm text-muted text-pretty">{t('facc.inheritedOnly')}</p>
				{:else}
					<div class="mt-2 rounded-box border border-base-content/10 p-5 text-center">
						<p class="text-sm font-semibold">{t('facc.empty.title')}</p>
						<p class="mx-auto mt-1 max-w-[48ch] text-sm text-muted text-pretty">
							{t('facc.empty.body')}
						</p>
					</div>
				{/if}

				{#if adding}
					<form
						method="POST"
						action="?/setAccess"
						use:enhance={submitAdd}
						class="mt-3 rounded-box border border-base-content/10 p-3"
					>
						<div class="min-w-[9rem]">
							<label class="text-xs font-medium" for="facc-add-group">
								{t('facc.add.group')}
							</label>
							<select
								id="facc-add-group"
								name="groupId"
								bind:value={addGroupId}
								use:focusHere
								class="select select-sm mt-1 w-full"
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
						</div>

						<div class="mt-3 pl-1">
							{@render capToggles(addCaps, (k, v) => setAddCap(k, v), false)}
						</div>

						<input type="hidden" name="canView" value={String(addCaps.can_view)} />
						<input type="hidden" name="canDownload" value={String(addCaps.can_download)} />
						<input type="hidden" name="canWatermark" value={String(addCaps.can_watermark)} />
						<input
							type="hidden"
							name="canDownloadOriginal"
							value={String(addCaps.can_download_original)}
						/>

						{#if addGroupId}
							{@const group = groups.find((g) => g.id === addGroupId)?.name ?? ''}
							{@const escalating = raises(addInherits ? capsOf(addInherits) : null, addCaps)}
							<div
								class="mt-3 rounded-field border p-2.5
									{escalating ? 'border-warning/50 bg-warning/8' : 'border-base-content/10'}"
							>
								<p class="text-xs text-pretty">{consequence(group, addCaps)}</p>
								<p class="mt-1 text-[0.6875rem] text-muted">{t('facc.cap.rule')}</p>
							</div>
						{/if}

						{#if formError && errorScope === 'add'}
							<div class="mt-3"><Alert align="start">{formError}</Alert></div>
						{/if}

						<div class="mt-3 flex justify-end gap-2">
							<Button type="button" variant="ghost" size="sm" onclick={() => (adding = false)}>
								{t('facc.add.cancel')}
							</Button>
							<Button type="submit" size="sm" loading={addSubmitting} disabled={!addGroupId}>
								{addInherits ? t('facc.add.change') : t('facc.add.submit')}
							</Button>
						</div>
					</form>
				{:else if addable.length}
					<button
						type="button"
						onclick={startAdd}
						class="mt-3 inline-flex items-center gap-1.5 rounded-field px-2 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
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
					<p class="mt-3 text-xs text-muted">{t('facc.allGranted')}</p>
				{/if}
			</section>

			{#if inherited.length}
				<section class="mt-6">
					<div class="flex items-baseline justify-between gap-2">
						<h3 class="text-sm font-semibold text-muted">{t('facc.inherited')}</h3>
						<span class="font-mono text-xs text-muted">
							{t('facc.inheritedCount', { n: inherited.length })}
						</span>
					</div>
					<ul class="mt-1 divide-y divide-base-content/8">
						{#each inherited as row (row.group_id)}
							{@const blocking = confirmBlock === row.group_id}
							<li class="py-2">
								<div class="flex items-center gap-2">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-medium" title={row.group_name}>
											{row.group_name}
										</p>
										<p class="mt-0.5 truncate text-xs text-muted">
											{t('facc.inheritedFrom', { name: row.source_folder_name })}
										</p>
									</div>
									<span class="flex-none text-sm font-medium">{capsLabel(row)}</span>
									{#if row.can_watermark}{@render wmMark(row.can_download)}{/if}
									{#if row.can_download_original}{@render origMark()}{/if}
									<button
										type="button"
										onclick={() =>
											(confirmBlock = confirmBlock === row.group_id ? null : row.group_id)}
										aria-expanded={blocking}
										aria-label={t('facc.blockOf', { group: row.group_name })}
										class="flex-none rounded-field px-2 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-error/10 hover:text-error"
									>
										{t('facc.block')}
									</button>
									<button
										type="button"
										onclick={() => startOverride(row)}
										aria-label={t('facc.overrideOf', { group: row.group_name })}
										class="flex-none rounded-field px-2 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
									>
										{t('facc.override')}
									</button>
								</div>

								{#if blocking}
									<form
										method="POST"
										action="?/setAccess"
										use:enhance={submitBlock(row)}
										class="mt-2 rounded-field border border-base-content/10 p-2.5"
									>
										<input type="hidden" name="groupId" value={row.group_id} />
										<input type="hidden" name="canView" value="false" />
										<input type="hidden" name="canDownload" value="false" />
										<input type="hidden" name="canWatermark" value="false" />
										<input type="hidden" name="canDownloadOriginal" value="false" />
										<p class="text-xs text-pretty" tabindex="-1" use:focusHere>
											{consequence(row.group_name, BLOCKED)}
										</p>
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
								{/if}

								{#if formError && errorScope === row.group_id}
									<div class="mt-2"><Alert align="start">{formError}</Alert></div>
								{/if}
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/if}

		<p aria-live="polite" class="mt-3 text-xs text-muted text-pretty">{status ?? ''}</p>
	</div>
</section>

<style>
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
