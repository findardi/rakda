<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Toaster, showToast } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { t } from '$lib/i18n';
	import type { GroupMemberData, WorkspaceMemberData } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const group = $derived(data.group);
	const members = $derived(data.members);
	const isDefault = $derived(group.is_default);
	const defaultName = $derived(data.defaultGroup?.name ?? '');
	const canReturn = $derived(!isDefault && !!data.defaultGroup);

	const base = $derived(`/workspace/${page.params.slug}/management-access/group`);

	const initial = (name: string, email: string) => (name || email || '?').charAt(0).toUpperCase();

	const assignedIds = $derived(new Set(members.map((m) => m.member_id)));
	const candidates = $derived(
		data.workspaceMembers.filter((m) => m.role_name === 'guest' && !assignedIds.has(m.id))
	);
	const guestTotal = $derived(data.workspaceMembers.filter((m) => m.role_name === 'guest').length);
	const memberBase = $derived(`/workspace/${page.params.slug}/management-access/member`);

	const currentGroupOf = (m: WorkspaceMemberData) => m.group_names?.[0] ?? '';

	// --- Assign dialog ---
	let assignDialog = $state<HTMLDialogElement>();
	let query = $state('');
	let selected = $state<Record<string, boolean>>({});
	let assignSubmitting = $state(false);
	let assignMessage = $state<string | null>(null);

	const selectedIds = $derived(Object.keys(selected).filter((id) => selected[id]));
	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return candidates;
		return candidates.filter((m) => `${m.username} ${m.email}`.toLowerCase().includes(q));
	});

	function openAssign() {
		query = '';
		selected = {};
		assignMessage = null;
		assignDialog?.showModal();
	}

	function toggle(m: WorkspaceMemberData) {
		selected = { ...selected, [m.id]: !selected[m.id] };
	}

	const submitAssign: SubmitFunction = () => {
		assignSubmitting = true;
		return async ({ result }) => {
			assignSubmitting = false;
			if (result.type === 'success') {
				const n = (result.data?.assigned as number) ?? selectedIds.length;
				assignDialog?.close();
				await invalidateAll();
				showToast(t('group.move.toast', { n, name: group.name }), 'success');
			} else if (result.type === 'failure') {
				assignMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				assignMessage = t('err.generic');
			}
		};
	};

	// --- Q&A settings (own PATCH action; not part of the name/description PUT) ---
	let qaEnabled = $state(false);
	let qaLimit = $state<number | ''>('');
	let qaSubmitting = $state(false);
	let qaMessage = $state<string | null>(null);
	let lastQaGroup: typeof data.group | null = null;

	$effect(() => {
		if (lastQaGroup === data.group) return;
		lastQaGroup = data.group;
		qaEnabled = data.group.qa_enabled;
		qaLimit = data.group.qa_question_limit ?? '';
	});

	// Idle saves would still PATCH and mint a spurious qa_settings_changed audit
	// row, so the button stays off until something actually changed.
	const qaLimitNorm = $derived(typeof qaLimit === 'number' ? qaLimit : null);
	const qaDirty = $derived(
		qaEnabled !== data.group.qa_enabled || qaLimitNorm !== (data.group.qa_question_limit ?? null)
	);

	const submitQa: SubmitFunction = () => {
		qaSubmitting = true;
		return async ({ result }) => {
			qaSubmitting = false;
			if (result.type === 'success') {
				qaMessage = null;
				await invalidateAll();
				showToast(t('qa.group.saved'), 'success');
			} else if (result.type === 'failure') {
				qaMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				qaMessage = t('err.generic');
			}
		};
	};

	let unassigningId = $state<string | null>(null);
	const submitUnassign = (m: GroupMemberData): SubmitFunction => {
		return () => {
			unassigningId = m.member_id;
			return async ({ result }) => {
				unassigningId = null;
				if (result.type === 'success') {
					await invalidateAll();
					showToast(
						t('group.back.toast', { member: m.username || m.email, name: defaultName }),
						'success'
					);
				} else if (result.type === 'failure') {
					showToast((result.data?.message as string) ?? t('err.generic'), 'error');
				} else {
					showToast(t('err.generic'), 'error');
				}
			};
		};
	};
</script>

<svelte:head><title>{group.name} · {t('ma.group')}</title></svelte:head>

<a
	href={base}
	class="inline-flex items-center gap-1.5 text-sm font-medium text-muted transition-colors hover:text-base-content"
>
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
		<path d="m15 18-6-6 6-6" />
	</svg>
	{t('group.detail.back')}
</a>

<div class="mt-4 flex flex-wrap items-start justify-between gap-4">
	<div class="min-w-0">
		<div class="flex flex-wrap items-center gap-2">
			<h1 class="text-xl font-semibold tracking-[-0.01em] text-balance">{group.name}</h1>
			{#if isDefault}
				<span
					class="rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
				>
					{t('group.default.badge')}
				</span>
			{/if}
		</div>
		{#if isDefault}
			<p class="mt-1 max-w-[60ch] text-sm text-muted text-pretty">{t('group.default.note')}</p>
		{:else if group.description}
			<p class="mt-1 max-w-[60ch] text-sm text-muted text-pretty">{group.description}</p>
		{/if}
		<p class="mt-1.5 text-xs text-muted">
			{members.length === 0
				? t('group.detail.countEmpty')
				: t('group.detail.count', { n: members.length })}
		</p>
	</div>

	<button type="button" onclick={openAssign} class="btn btn-primary btn-sm flex-none">
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
			<path d="M5 12h14M13 6l6 6-6 6" />
		</svg>
		{t('group.move')}
	</button>
</div>

{#if canReturn}
	<p class="mt-4 text-xs text-muted text-pretty">{t('group.back.note', { name: defaultName })}</p>
{/if}

<section class="mt-6 rounded-box border border-base-content/10 bg-base-100 px-4 py-4">
	<h2 class="text-sm font-semibold">{t('qa.group.title')}</h2>

	{#if qaMessage}
		<div class="mt-3"><Alert align="start">{qaMessage}</Alert></div>
	{/if}

	<form
		method="POST"
		action="?/qa"
		use:enhance={submitQa}
		class="mt-3 flex flex-wrap items-end gap-x-6 gap-y-3"
	>
		<label class="flex cursor-pointer items-center gap-2">
			<input
				type="checkbox"
				name="qa_enabled"
				class="checkbox checkbox-sm"
				bind:checked={qaEnabled}
			/>
			<span class="text-xs font-medium">{t('qa.group.enabled')}</span>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('qa.group.limit')}</span>
			<input
				type="number"
				name="question_limit"
				min="0"
				step="1"
				placeholder="∞"
				class="input input-sm w-24 font-mono"
				bind:value={qaLimit}
			/>
		</label>

		<Button type="submit" loading={qaSubmitting} disabled={!qaDirty}>
			{qaSubmitting ? t('qa.group.saving') : t('qa.group.save')}
		</Button>

		<div class="w-full text-xs text-muted">
			<p>{t('qa.group.enabledHint')}</p>
			<p class="mt-0.5">{t('qa.group.limitHint')}</p>
		</div>
	</form>
</section>

{#if members.length}
	<ul class="mt-6 divide-y divide-base-content/10 border-y border-base-content/10">
		{#each members as m (m.member_id)}
			{@const busy = unassigningId === m.member_id}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
				<span
					class="grid h-9 w-9 flex-none place-items-center rounded-full bg-primary/10 text-sm font-semibold text-primary"
					aria-hidden="true">{initial(m.username, m.email)}</span
				>

				<div class="min-w-0 flex-1 basis-48">
					<div class="flex items-center gap-2">
						<span class="truncate text-[0.9375rem] font-medium">{m.username || m.email}</span>
						<span
							class="rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
							>{roleDisplayName(m.role_name)}</span
						>
					</div>
					<p class="mt-0.5 truncate font-mono text-xs text-muted">{m.email}</p>
				</div>

				{#if canReturn}
					<form method="POST" action="?/unassign" use:enhance={submitUnassign(m)} class="contents">
						<input type="hidden" name="memberId" value={m.member_id} />
						<button
							type="submit"
							disabled={busy}
							aria-label={t('group.back.actionOf', {
								member: m.username || m.email,
								name: defaultName
							})}
							class="inline-flex flex-none items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-50"
						>
							{#if busy}
								<span class="loading loading-spinner loading-xs"></span>
							{:else}
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
									<path d="M19 12H5M11 18l-6-6 6-6" />
								</svg>
							{/if}
							{t('group.back.action', { name: defaultName })}
						</button>
					</form>
				{/if}
			</li>
		{/each}
	</ul>
{:else}
	<div class="mt-6 rounded-box border border-dashed border-base-content/15 px-6 py-12 text-center">
		<span
			class="mx-auto grid h-11 w-11 place-items-center rounded-full bg-base-content/5 text-muted"
			aria-hidden="true"
		>
			<svg
				class="h-5 w-5"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"
			>
				<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
				<circle cx="9" cy="7" r="4" />
				<path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
			</svg>
		</span>
		<h3 class="mt-3 text-sm font-semibold">{t('group.detail.empty.title')}</h3>
		<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
			{t('group.detail.empty.body')}
		</p>
		<button type="button" onclick={openAssign} class="btn btn-primary btn-sm mt-4">
			{t('group.move')}
		</button>
	</div>
{/if}

<!-- Assign members -->
<dialog bind:this={assignDialog} class="modal" aria-labelledby="group-assign-title">
	<div
		class="modal-box flex max-h-[80vh] max-w-md flex-col rounded-box border border-base-content/10 bg-base-100 p-6"
	>
		<h2 id="group-assign-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('group.move')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">
			{t('group.move.desc', { name: group.name })}
		</p>

		{#if assignMessage}
			<div class="mt-4"><Alert align="start">{assignMessage}</Alert></div>
		{/if}

		{#if candidates.length}
			<div class="relative mt-4">
				<svg
					class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<circle cx="11" cy="11" r="7" />
					<path d="m21 21-4.3-4.3" />
				</svg>
				<input
					type="search"
					bind:value={query}
					placeholder={t('group.assign.search')}
					aria-label={t('group.assign.search')}
					class="input w-full pl-9 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
				/>
			</div>

			<form
				method="POST"
				action="?/assign"
				use:enhance={submitAssign}
				class="mt-4 flex min-h-0 flex-1 flex-col"
			>
				{#each selectedIds as id (id)}
					<input type="hidden" name="memberId" value={id} />
				{/each}

				<ul class="-mx-1 min-h-0 flex-1 overflow-y-auto">
					{#each filtered as m (m.id)}
						{@const checked = !!selected[m.id]}
						<li>
							<label
								class="flex cursor-pointer items-center gap-3 rounded-field px-1 py-2 transition-colors hover:bg-base-content/4"
							>
								<input
									type="checkbox"
									{checked}
									onchange={() => toggle(m)}
									class="checkbox checkbox-sm checkbox-primary flex-none"
								/>
								<span
									class="grid h-8 w-8 flex-none place-items-center rounded-full bg-base-content/10 text-xs font-semibold text-muted"
									aria-hidden="true">{initial(m.username, m.email)}</span
								>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium">{m.username || m.email}</span>
									<span class="block truncate font-mono text-xs text-muted">{m.email}</span>
									{#if currentGroupOf(m)}
										<span class="mt-0.5 block truncate text-xs text-muted">
											{t('group.move.currentGroup', { name: currentGroupOf(m) })}
										</span>
									{/if}
								</span>
								<span
									class="flex-none rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
									>{roleDisplayName(m.role_name)}</span
								>
							</label>
						</li>
					{:else}
						<li class="px-1 py-6 text-center text-sm text-muted">{t('group.assign.noMatch')}</li>
					{/each}
				</ul>

				<div
					class="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-base-content/10 pt-4"
				>
					<span class="text-sm text-muted" aria-live="polite">
						{t('group.assign.selected', { n: selectedIds.length })}
					</span>
					<div class="flex gap-2">
						<Button type="button" variant="ghost" onclick={() => assignDialog?.close()}>
							{t('group.cancel')}
						</Button>
						<Button type="submit" loading={assignSubmitting} disabled={selectedIds.length === 0}>
							{assignSubmitting ? t('group.move.submitting') : t('group.move.submit')}
						</Button>
					</div>
				</div>
			</form>
		{:else if guestTotal === 0}
			<div class="mt-6 rounded-box bg-base-content/4 px-6 py-8 text-center">
				<p class="text-sm font-medium">{t('group.assign.noGuests.title')}</p>
				<p class="mx-auto mt-1 max-w-xs text-sm text-muted text-pretty">
					{t('group.assign.noGuests.body')}
				</p>
				<a href={memberBase} class="btn btn-primary btn-sm mt-4">
					{t('group.assign.noGuests.cta')}
				</a>
			</div>
			<div class="mt-5 flex justify-end">
				<Button type="button" variant="ghost" onclick={() => assignDialog?.close()}>
					{t('group.detail.done')}
				</Button>
			</div>
		{:else}
			<div class="mt-6 rounded-box bg-base-content/4 px-6 py-8 text-center">
				<p class="text-sm font-medium">{t('group.move.allIn.title')}</p>
				<p class="mx-auto mt-1 max-w-xs text-sm text-muted text-pretty">
					{t('group.move.allIn.body')}
				</p>
			</div>
			<div class="mt-5 flex justify-end">
				<Button type="button" variant="ghost" onclick={() => assignDialog?.close()}>
					{t('group.detail.done')}
				</Button>
			</div>
		{/if}
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('group.cancel')}></button>
	</form>
</dialog>

<Toaster />
