<script lang="ts">
	import { getContext } from 'svelte';
	import { enhance } from '$app/forms';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, showToast } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { formatDateLocal } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { InvitationData } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const invitations = $derived(data.invitations);

	const invite = getContext<{ open: () => void }>('member-invite');

	const initial = (inv: InvitationData) => (inv.email || '?').charAt(0).toUpperCase();

	// Status → label + color (full class strings keep Tailwind JIT happy).
	const statusMeta = (s: string) => {
		switch (s) {
			case 'pending':
				return {
					label: t('inv.status.pending'),
					dot: 'bg-warning',
					avatar: 'bg-warning/10 text-warning'
				};
			case 'accepted':
				return {
					label: t('inv.status.accepted'),
					dot: 'bg-success',
					avatar: 'bg-success/10 text-success'
				};
			case 'expired':
				return {
					label: t('inv.status.expired'),
					dot: 'bg-base-content/40',
					avatar: 'bg-base-content/10 text-base-content/60'
				};
			case 'revoked':
				return {
					label: t('inv.status.revoked'),
					dot: 'bg-error',
					avatar: 'bg-error/10 text-error'
				};
			case 'rejected':
				return {
					label: t('inv.status.rejected'),
					dot: 'bg-error',
					avatar: 'bg-error/10 text-error'
				};
			default:
				return { label: s, dot: 'bg-base-content/30', avatar: 'bg-base-content/10 text-muted' };
		}
	};
	// Resend/revoke only make sense while an invite is still live.
	const canManage = (s: string) => s === 'pending' || s === 'expired';

	const filters = ['pending', 'accepted', 'expired', 'revoked', 'rejected', 'all'];
	const filterLabel = (f: string) => (f === 'all' ? t('inv.filter.all') : statusMeta(f).label);
	function setFilter(value: string) {
		const u = new URL(page.url);
		u.searchParams.set('status', value);
		goto(u, { keepFocus: true, noScroll: true });
	}

	// Reveal long histories incrementally instead of one flat wall of rows.
	let limit = $state(25);
	const shown = $derived(invitations.slice(0, limit));

	// --- Resend (direct, per row) ---
	let resendingId = $state<string | null>(null);
	const submitResend = (id: string): SubmitFunction => {
		return () => {
			resendingId = id;
			return async ({ result }) => {
				resendingId = null;
				if (result.type === 'success') {
					await invalidateAll();
					showToast(t('pending.resend.done'), 'success');
				} else if (result.type === 'failure') {
					showToast((result.data?.message as string) ?? t('err.generic'), 'error');
				} else {
					showToast(t('err.generic'), 'error');
				}
			};
		};
	};

	// --- Revoke (confirm, destructive) ---
	let revokeDialog = $state<HTMLDialogElement>();
	let revokeTarget = $state<InvitationData | null>(null);
	let revokeSubmitting = $state(false);
	let revokeMessage = $state<string | null>(null);

	function openRevoke(inv: InvitationData) {
		revokeTarget = inv;
		revokeMessage = null;
		revokeDialog?.showModal();
	}

	const submitRevoke: SubmitFunction = () => {
		revokeSubmitting = true;
		return async ({ result }) => {
			revokeSubmitting = false;
			if (result.type === 'success') {
				revokeDialog?.close();
				await invalidateAll();
				showToast(t('pending.revoke.done'), 'success');
			} else if (result.type === 'failure') {
				revokeMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				revokeMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head><title>{t('ma.pending')} · {t('ma.title')}</title></svelte:head>

<div>
	<label class="sr-only" for="inv-filter">{t('inv.filter.label')}</label>
	<select
		id="inv-filter"
		value={data.status}
		onchange={(e) => setFilter(e.currentTarget.value)}
		class="select select-sm w-auto"
		aria-label={t('inv.filter.label')}
	>
		{#each filters as f (f)}
			<option value={f}>{filterLabel(f)}</option>
		{/each}
	</select>
</div>

{#if invitations.length}
	<ul class="mt-4 divide-y divide-base-content/10 border-y border-base-content/10">
		{#each shown as inv (inv.id)}
			{@const meta = statusMeta(inv.status)}
			{@const manageable = canManage(inv.status)}
			{@const resending = resendingId === inv.id}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
				<span
					class="grid h-9 w-9 flex-none place-items-center rounded-full text-sm font-semibold {meta.avatar}"
					aria-hidden="true">{initial(inv)}</span
				>

				<div class="min-w-0 flex-1 basis-48">
					<div class="flex items-center gap-2">
						<span class="truncate font-mono text-[0.9375rem] font-medium">{inv.email}</span>
						<span
							class="rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
							>{roleDisplayName(inv.role_name)}</span
						>
						{#if inv.group_name}
							<span
								title={t('pending.group')}
								class="rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
								>{inv.group_name}</span
							>
						{/if}
					</div>
					<p class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted">
						<span class="inline-flex items-center gap-1.5">
							<span class="h-1.5 w-1.5 rounded-full {meta.dot}"></span>
							{meta.label}
						</span>
						<span aria-hidden="true">·</span>
						<span>
							{t('pending.expires')}
							<span class="font-mono">{formatDateLocal(inv.expires_at)}</span>
						</span>
						{#if inv.access_expires_at}
							<span aria-hidden="true">·</span>
							<span>
								{t('pending.accessUntil')}
								<span class="font-mono">{formatDateLocal(inv.access_expires_at)}</span>
							</span>
						{/if}
						{#if inv.invited_by_username}
							<span aria-hidden="true">·</span>
							<span>{t('pending.invitedBy', { name: inv.invited_by_username })}</span>
						{/if}
					</p>
				</div>

				{#if manageable}
					<div class="flex flex-none items-center gap-1">
						<form
							method="POST"
							action="?/resend"
							use:enhance={submitResend(inv.id)}
							class="contents"
						>
							<input type="hidden" name="invitationId" value={inv.id} />
							<button
								type="submit"
								disabled={resending}
								class="inline-flex items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:pointer-events-none disabled:opacity-50"
							>
								{#if resending}
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
										<path d="M21 2v6h-6" />
										<path d="M3 12a9 9 0 0 1 15-6.7L21 8" />
										<path d="M3 22v-6h6" />
										<path d="M21 12a9 9 0 0 1-15 6.7L3 16" />
									</svg>
								{/if}
								{t('pending.resend')}
							</button>
						</form>
						<button
							type="button"
							onclick={() => openRevoke(inv)}
							class="inline-flex items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm text-muted transition-colors hover:bg-error/10 hover:text-error"
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
								<circle cx="12" cy="12" r="9" />
								<path d="m15 9-6 6M9 9l6 6" />
							</svg>
							{t('pending.revoke.short')}
						</button>
					</div>
				{/if}
			</li>
		{/each}
	</ul>

	{#if invitations.length > limit}
		<div class="mt-4 flex justify-center">
			<button
				type="button"
				onclick={() => (limit += 25)}
				class="text-sm font-medium text-primary hover:underline"
			>
				{t('list.more', { n: invitations.length - limit })}
			</button>
		</div>
	{/if}
{:else if data.status === 'pending' || data.status === 'all'}
	<div
		class="mt-4 grid place-items-center rounded-box border border-dashed border-base-content/15 px-6 py-14 text-center"
	>
		<span
			class="grid h-11 w-11 place-items-center rounded-full bg-base-content/5 text-muted"
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
				<rect x="3" y="5" width="18" height="14" rx="2" />
				<path d="m3 7 9 6 9-6" />
			</svg>
		</span>
		<h3 class="mt-3 text-sm font-semibold">{t('pending.empty.title')}</h3>
		<p class="mt-1 max-w-sm text-sm text-muted text-pretty">{t('pending.empty.desc')}</p>
		<button type="button" onclick={invite.open} class="btn btn-primary btn-sm mt-4">
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
				<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
				<circle cx="9" cy="7" r="4" />
				<path d="M19 8v6M22 11h-6" />
			</svg>
			{t('member.invite')}
		</button>
	</div>
{:else}
	<div
		class="mt-4 flex flex-col items-center gap-3 rounded-box border border-dashed border-base-content/15 px-6 py-12 text-center"
	>
		<p class="text-sm text-muted">{t('inv.empty.filtered')}</p>
		<button
			type="button"
			onclick={() => setFilter('pending')}
			class="text-sm font-medium text-primary hover:underline"
		>
			{t('inv.empty.reset')}
		</button>
	</div>
{/if}

<!-- Revoke confirm -->
<dialog bind:this={revokeDialog} class="modal" aria-labelledby="invite-revoke-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="invite-revoke-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('pending.revoke.title')}
		</h2>
		{#if revokeTarget}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('pending.revoke.warning', { email: revokeTarget.email })}
			</p>
		{/if}

		{#if revokeMessage}
			<div class="mt-4"><Alert align="start">{revokeMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/revoke"
			use:enhance={submitRevoke}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="invitationId" value={revokeTarget?.id ?? ''} />
			<Button type="button" variant="ghost" onclick={() => revokeDialog?.close()}>
				{t('member.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={revokeSubmitting}>
				{revokeSubmitting ? t('pending.revoke.submitting') : t('pending.revoke.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('member.cancel')}></button>
	</form>
</dialog>
