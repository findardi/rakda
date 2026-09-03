<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, showToast } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { assignableRoles, canManageMembers } from '$lib/access/roles';
	import { formatDateLocal, localDayEnd, localDayString, localTomorrowString } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { MemberStatus, MyAccessWorkspace, WorkspaceMemberData } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const members = $derived(data.members);
	const viewerRole = $derived((page.data as { access?: MyAccessWorkspace }).access?.role ?? '');
	const canManage = $derived(canManageMembers(viewerRole));
	// Roles the viewer may grant: owner → all but owner; admin → guest only.
	const roleOptions = $derived(assignableRoles(viewerRole, data.roles));

	const isOwner = (m: WorkspaceMemberData) => m.user_id === data.ownerId;
	// The signed-in user can't remove their own membership from the list.
	const selfId = $derived((page.data as { user?: { id: string } }).user?.id ?? '');
	const isSelf = (m: WorkspaceMemberData) => m.user_id === selfId;
	// A member's role is changeable only if its current role is one the viewer may
	// grant — so an admin can't demote a fellow admin (the backend rejects it too).
	const canChangeRole = (m: WorkspaceMemberData) =>
		canManage && !isOwner(m) && roleOptions.some((r) => r.id === m.role_id);
	const initial = (m: WorkspaceMemberData) =>
		(m.username || m.email || '?').charAt(0).toUpperCase();

	const statusMeta = (s: MemberStatus) => {
		if (s === 'active') return { label: t('member.status.active'), dot: 'bg-success' };
		if (s === 'invited') return { label: t('member.status.invited'), dot: 'bg-warning' };
		if (s === 'suspended') return { label: t('member.status.suspended'), dot: 'bg-error' };
		if (s === 'expired') return { label: t('member.status.expired'), dot: 'bg-error' };
		return { label: s, dot: 'bg-base-content/30' };
	};

	// Per-member select value seeded from server truth; reverts on a cancelled or
	// failed change because any reload re-derives it.
	let choice = $derived(Object.fromEntries(data.members.map((m) => [m.id, m.role_id])));

	// --- Search + status filter (client-side; the list is already fully loaded) ---
	let query = $state('');
	let statusFilter = $state<'all' | MemberStatus>('all');
	const statusFilters: MemberStatus[] = ['active', 'invited', 'suspended', 'expired'];
	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return members.filter((m) => {
			if (statusFilter !== 'all' && m.status !== statusFilter) return false;
			if (!q) return true;
			return `${m.username ?? ''} ${m.email ?? ''}`.toLowerCase().includes(q);
		});
	});

	// Reveal long lists incrementally rather than dumping every row at once.
	let limit = $state(25);
	const shown = $derived(filtered.slice(0, limit));

	// --- Change role (confirm before applying; permission changes are consequential) ---
	let roleDialog = $state<HTMLDialogElement>();
	let roleChange = $state<{ member: WorkspaceMemberData; toRoleId: string; toName: string } | null>(
		null
	);
	let roleSubmitting = $state(false);
	let roleMessage = $state<string | null>(null);

	function requestRoleChange(m: WorkspaceMemberData, toRoleId: string) {
		if (toRoleId === m.role_id) return;
		const toName = roleDisplayName(roleOptions.find((r) => r.id === toRoleId)?.name ?? '');
		roleChange = { member: m, toRoleId, toName };
		roleMessage = null;
		roleDialog?.showModal();
	}

	function cancelRoleChange() {
		if (roleChange) choice[roleChange.member.id] = roleChange.member.role_id; // revert the select
		roleDialog?.close();
	}

	const submitRoleChange: SubmitFunction = () => {
		roleSubmitting = true;
		return async ({ result }) => {
			roleSubmitting = false;
			if (result.type === 'success') {
				roleDialog?.close();
				await invalidateAll();
				showToast(t('member.roleChanged'), 'success');
			} else if (result.type === 'failure') {
				roleMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				roleMessage = t('err.generic');
			}
		};
	};

	// --- Access expiry (guests only; the backend refuses other roles) ---
	let expiryDialog = $state<HTMLDialogElement>();
	let expiryForm = $state<HTMLFormElement>();
	let expiryTarget = $state<WorkspaceMemberData | null>(null);
	let expiryDate = $state('');
	let minExpiryDate = $state('');
	let expiryClear = $state(false);
	let expirySubmitting = $state(false);
	let expiryMessage = $state<string | null>(null);

	function openExpiry(m: WorkspaceMemberData) {
		expiryTarget = m;
		expiryDate = m.expires_at ? localDayString(new Date(m.expires_at)) : '';
		minExpiryDate = localTomorrowString();
		expiryClear = false;
		expiryMessage = null;
		expiryDialog?.showModal();
	}

	// A type="button" that submits programmatically, so pressing Enter in the
	// date field can never fall through to "clear".
	function clearExpiry() {
		expiryClear = true;
		expiryForm?.requestSubmit();
	}

	const submitExpiry: SubmitFunction = ({ formData, cancel }) => {
		const clear = expiryClear;
		expiryClear = false;
		if (clear) {
			formData.set('expiresAt', '');
		} else if (expiryDate) {
			// End of the chosen day in this browser's timezone — see InviteDialog.
			formData.set('expiresAt', localDayEnd(expiryDate).toISOString());
		} else {
			cancel();
			return;
		}
		expirySubmitting = true;
		expiryMessage = null;
		return async ({ result }) => {
			expirySubmitting = false;
			if (result.type === 'success') {
				expiryDialog?.close();
				await invalidateAll();
				showToast(t('member.expiry.saved'), 'success');
			} else if (result.type === 'failure') {
				expiryMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				expiryMessage = t('err.generic');
			}
		};
	};

	// --- Remove ---
	let removeDialog = $state<HTMLDialogElement>();
	let pending = $state<WorkspaceMemberData | null>(null);
	let removeSubmitting = $state(false);
	let removeMessage = $state<string | null>(null);

	function openRemove(m: WorkspaceMemberData) {
		pending = m;
		removeMessage = null;
		removeDialog?.showModal();
	}

	const submitRemove: SubmitFunction = () => {
		removeSubmitting = true;
		return async ({ result }) => {
			removeSubmitting = false;
			if (result.type === 'success') {
				removeDialog?.close();
				await invalidateAll();
				showToast(t('member.removed'), 'success');
			} else if (result.type === 'failure') {
				removeMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				removeMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head><title>{t('ma.member')} · {t('ma.title')}</title></svelte:head>

<div class="mb-4 flex flex-wrap items-center gap-2">
	<div class="relative min-w-0 flex-1 basis-56">
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
			placeholder={t('member.search')}
			aria-label={t('member.search')}
			class="input input-sm w-full pl-9"
		/>
	</div>
	<select
		bind:value={statusFilter}
		aria-label={t('member.filter.all')}
		class="select select-sm flex-none"
	>
		<option value="all">{t('member.filter.all')}</option>
		{#each statusFilters as s (s)}
			<option value={s}>{statusMeta(s).label}</option>
		{/each}
	</select>
</div>

{#if shown.length === 0}
	<p class="border-y border-base-content/10 py-10 text-center text-sm text-muted">
		{t('member.noMatch')}
	</p>
{:else}
	<ul class="divide-y divide-base-content/10 border-y border-base-content/10">
		{#each shown as m (m.id)}
			{@const status = statusMeta(m.status)}
			{@const owner = isOwner(m)}
			{@const memberGroup = m.group_names?.[0] ?? ''}
			{@const guest = m.role_name === 'guest'}
			{@const expired = m.status === 'expired'}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
				<span
					class="grid h-9 w-9 flex-none place-items-center rounded-full bg-primary/10 text-sm font-semibold text-primary"
					aria-hidden="true">{initial(m)}</span
				>

				<div class="min-w-0 flex-1 basis-48">
					<div class="flex items-center gap-2">
						<span class="truncate text-[0.9375rem] font-medium">{m.username || m.email}</span>
						{#if owner}
							<span
								class="rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
								>{roleDisplayName('owner')}</span
							>
						{/if}
					</div>
					<p class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted">
						<span class="truncate font-mono">{m.email}</span>
						<span aria-hidden="true">·</span>
						<span class="inline-flex items-center gap-1.5">
							<span class="h-1.5 w-1.5 rounded-full {status.dot}"></span>
							{status.label}
						</span>
					</p>
					{#if guest}
						<div class="mt-1.5 flex flex-wrap items-center gap-1.5">
							{#if memberGroup}
								<span
									class="rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
									>{memberGroup}</span
								>
							{:else}
								<span class="text-[0.6875rem] text-muted">{t('group.member.none')}</span>
							{/if}
							{#if m.expires_at}
								<span
									class="rounded-selector px-1.5 py-0.5 text-[0.6875rem] {expired
										? 'bg-error/10 text-error'
										: 'bg-base-content/5 text-muted'}"
								>
									{expired ? t('member.expiry.lapsed') : t('member.expiry.until')}
									<span class="font-mono">{formatDateLocal(m.expires_at)}</span>
								</span>
							{/if}
						</div>
					{/if}
				</div>

				{#if canChangeRole(m)}
					<select
						bind:value={choice[m.id]}
						onchange={(e) => requestRoleChange(m, e.currentTarget.value)}
						aria-label={t('member.changeRole', { name: m.username || m.email })}
						class="select select-sm w-36 flex-none"
					>
						{#each roleOptions as r (r.id)}
							<option value={r.id}>{roleDisplayName(r.name)}</option>
						{/each}
					</select>
				{:else}
					<span
						class="inline-flex flex-none items-center gap-1.5 text-sm text-muted"
						title={owner ? t('member.role.locked') : undefined}
					>
						{roleDisplayName(m.role_name)}
						<svg
							class="h-3.5 w-3.5 flex-none"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.8"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<rect x="5" y="11" width="14" height="9" rx="2" />
							<path d="M8 11V7a4 4 0 0 1 8 0v4" />
						</svg>
					</span>
				{/if}

				{#if canManage && guest}
					<button
						type="button"
						onclick={() => openExpiry(m)}
						aria-label={t('member.expiry.aria', { name: m.username || m.email })}
						class="inline-flex flex-none items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm text-muted transition-colors hover:bg-primary/10 hover:text-primary"
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
							<rect x="3" y="5" width="18" height="16" rx="2" />
							<path d="M16 3v4M8 3v4M3 11h18" />
						</svg>
						{t('member.expiry')}
					</button>
				{/if}

				{#if canManage && !owner && !isSelf(m)}
					<button
						type="button"
						onclick={() => openRemove(m)}
						aria-label={t('member.remove.aria', { name: m.username || m.email })}
						class="inline-flex flex-none items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm text-muted transition-colors hover:bg-error/10 hover:text-error"
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
							<path
								d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"
							/>
							<path d="M10 11v6M14 11v6" />
						</svg>
						{t('member.remove')}
					</button>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

{#if filtered.length > limit}
	<div class="mt-4 flex justify-center">
		<button
			type="button"
			onclick={() => (limit += 25)}
			class="text-sm font-medium text-primary hover:underline"
		>
			{t('list.more', { n: filtered.length - limit })}
		</button>
	</div>
{/if}

<!-- Change role confirm -->
<dialog
	bind:this={roleDialog}
	oncancel={cancelRoleChange}
	class="modal"
	aria-labelledby="member-role-title"
>
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="member-role-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('member.roleConfirm.title')}
		</h2>
		{#if roleChange}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('member.roleConfirm.warning', {
					name: roleChange.member.username || roleChange.member.email,
					from: roleDisplayName(roleChange.member.role_name),
					to: roleChange.toName
				})}
			</p>
		{/if}

		{#if roleMessage}
			<div class="mt-4"><Alert align="start">{roleMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/updateRole"
			use:enhance={submitRoleChange}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="memberId" value={roleChange?.member.id ?? ''} />
			<input type="hidden" name="roleId" value={roleChange?.toRoleId ?? ''} />
			<Button type="button" variant="ghost" onclick={cancelRoleChange}>{t('member.cancel')}</Button>
			<Button type="submit" loading={roleSubmitting}>
				{roleSubmitting ? t('member.roleConfirm.submitting') : t('member.roleConfirm.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('member.cancel')} onclick={cancelRoleChange}></button>
	</form>
</dialog>

<!-- Access expiry -->
<dialog bind:this={expiryDialog} class="modal" aria-labelledby="member-expiry-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="member-expiry-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('member.expiry.title')}
		</h2>
		{#if expiryTarget}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('member.expiry.desc', { name: expiryTarget.username || expiryTarget.email })}
			</p>
		{/if}

		{#if expiryMessage}
			<div class="mt-4"><Alert align="start">{expiryMessage}</Alert></div>
		{/if}

		<form
			bind:this={expiryForm}
			method="POST"
			action="?/updateExpiry"
			use:enhance={submitExpiry}
			class="mt-5"
		>
			<input type="hidden" name="memberId" value={expiryTarget?.id ?? ''} />
			<label class="text-sm font-medium" for="member-expiry-date"
				>{t('member.expiry.dateLabel')}</label
			>
			<input
				id="member-expiry-date"
				type="date"
				bind:value={expiryDate}
				min={minExpiryDate}
				class="input mt-1.5 w-full"
			/>
			<p class="mt-1.5 text-xs text-muted">{t('member.expiry.hint')}</p>

			<div class="mt-5 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => expiryDialog?.close()}>
					{t('member.cancel')}
				</Button>
				{#if expiryTarget?.expires_at}
					<Button
						type="button"
						variant="danger-outline"
						loading={expirySubmitting}
						onclick={clearExpiry}
					>
						{t('member.expiry.clear')}
					</Button>
				{/if}
				<Button type="submit" loading={expirySubmitting} disabled={!expiryDate}>
					{expirySubmitting ? t('member.expiry.submitting') : t('member.expiry.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('member.cancel')}></button>
	</form>
</dialog>

<!-- Remove confirm -->
<dialog bind:this={removeDialog} class="modal" aria-labelledby="member-remove-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="member-remove-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('member.remove.title')}
		</h2>
		{#if pending}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('member.remove.warning', { name: pending.username || pending.email })}
			</p>
		{/if}

		{#if removeMessage}
			<div class="mt-4"><Alert align="start">{removeMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/delete"
			use:enhance={submitRemove}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="memberId" value={pending?.id ?? ''} />
			<Button type="button" variant="ghost" onclick={() => removeDialog?.close()}>
				{t('member.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={removeSubmitting}>
				{removeSubmitting ? t('member.remove.submitting') : t('member.remove.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('member.cancel')}></button>
	</form>
</dialog>
