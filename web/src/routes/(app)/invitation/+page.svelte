<script lang="ts">
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Toaster, showToast } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { t } from '$lib/i18n';
	import type { MyInvitationData } from '$lib/types/invitation';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const invitations = $derived(data.invitations);

	const initial = (name: string) => (name || '?').charAt(0).toUpperCase();

	function expiry(iso: string): { label: string; urgent: boolean } {
		const ms = new Date(iso).getTime() - Date.now();
		const hours = ms / 3_600_000;
		const urgent = hours < 48;
		if (hours < 1) return { label: t('inv.expiresSoon'), urgent: true };
		if (hours < 24) {
			const n = Math.round(hours);
			return { label: t(n === 1 ? 'inv.expiresInHour' : 'inv.expiresInHours', { n }), urgent };
		}
		const n = Math.round(hours / 24);
		return { label: t(n === 1 ? 'inv.expiresInDay' : 'inv.expiresInDays', { n }), urgent };
	}

	let detailDialog = $state<HTMLDialogElement>();
	let selected = $state<MyInvitationData | null>(null);
	let acting = $state<null | 'accept' | 'reject'>(null);
	let detailMessage = $state<string | null>(null);

	function openDetail(inv: MyInvitationData) {
		selected = inv;
		detailMessage = null;
		acting = null;
		detailDialog?.showModal();
	}

	function submitAction(kind: 'accept' | 'reject'): SubmitFunction {
		return () => {
			const inv = selected;
			acting = kind;
			return async ({ result }) => {
				acting = null;
				if (result.type === 'success') {
					detailDialog?.close();
					await invalidateAll();
					showToast(
						kind === 'accept'
							? t('inv.accepted.toast', { name: inv?.workspace_name ?? '' })
							: t('inv.rejected.toast'),
						'success'
					);
				} else if (result.type === 'redirect') {
					await applyAction(result);
				} else if (result.type === 'failure') {
					detailMessage = (result.data?.message as string) ?? t('err.generic');
					await invalidateAll();
				} else {
					detailMessage = t('err.generic');
				}
			};
		};
	}
</script>

<svelte:head><title>{t('inv.title')} · {t('brand.name')}</title></svelte:head>

<div class="mx-auto w-full max-w-3xl px-6 py-8">
	<header class="flex items-start justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold tracking-[-0.015em]">{t('inv.title')}</h1>
			{#if invitations.length}
				<p class="mt-0.5 text-sm text-muted">{t('inv.count', { n: invitations.length })}</p>
			{/if}
		</div>
	</header>

	{#if data.loadError}
		<div class="mt-6"><Alert align="start">{t('inv.loadError')}</Alert></div>
	{:else if invitations.length === 0}
		<div class="mt-10 grid place-items-center px-6 py-16">
			<section class="flex max-w-md flex-col items-center text-center">
				<span
					class="mb-5 grid h-14 w-14 place-items-center rounded-box border border-base-content/10 bg-base-100 text-muted"
				>
					<svg
						class="h-6 w-6"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<rect x="3" y="5" width="18" height="14" rx="2" />
						<path d="m3 7 9 6 9-6" />
					</svg>
				</span>
				<h2 class="text-[1.375rem] font-semibold tracking-[-0.015em] text-balance">
					{t('inv.empty.title')}
				</h2>
				<p class="mt-2 max-w-[42ch] text-[0.9375rem] leading-relaxed text-muted text-pretty">
					{t('inv.empty.body')}
				</p>
			</section>
		</div>
	{:else}
		<ul
			class="mt-6 divide-y divide-base-content/10 overflow-hidden rounded-box border border-base-content/10 bg-base-100"
		>
			{#each invitations as inv (inv.id)}
				{@const exp = expiry(inv.expires_at)}
				<li>
					<button
						type="button"
						onclick={() => openDetail(inv)}
						aria-label={t('inv.open', { name: inv.workspace_name })}
						class="flex w-full items-center gap-4 px-4 py-4 text-left transition-colors hover:bg-base-content/5"
					>
						<span
							class="grid h-10 w-10 flex-none place-items-center rounded-field bg-primary/10 text-sm font-semibold text-primary"
							aria-hidden="true">{initial(inv.workspace_name)}</span
						>

						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
								<span class="truncate text-[0.9375rem] font-medium">{inv.workspace_name}</span>
								<span
									title={t('inv.role')}
									class="rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
								>
									{roleDisplayName(inv.role_name)}
								</span>
							</div>
							<p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted">
								<span class="truncate">{t('pending.invitedBy', { name: inv.invited_by })}</span>
								<span aria-hidden="true">·</span>
								<span class="inline-flex items-center gap-1 {exp.urgent ? 'text-warning' : ''}">
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
										<circle cx="12" cy="12" r="9" />
										<path d="M12 7v5l3 2" />
									</svg>
									{exp.label}
								</span>
							</p>
						</div>

						<svg
							class="h-4 w-4 flex-none text-muted"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.6"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="m9 6 6 6-6 6" />
						</svg>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<dialog bind:this={detailDialog} class="modal" aria-labelledby="inv-detail-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		{#if selected}
			{@const exp = expiry(selected.expires_at)}
			<div class="flex items-start gap-3">
				<span
					class="grid h-11 w-11 flex-none place-items-center rounded-field bg-primary/10 text-base font-semibold text-primary"
					aria-hidden="true">{initial(selected.workspace_name)}</span
				>
				<div class="min-w-0">
					<h2 id="inv-detail-title" class="truncate text-lg font-semibold tracking-[-0.01em]">
						{selected.workspace_name}
					</h2>
					<p class="mt-0.5 text-sm text-muted text-pretty">{t('inv.detail.subtitle')}</p>
				</div>
			</div>

			<dl
				class="mt-5 divide-y divide-base-content/10 overflow-hidden rounded-box border border-base-content/10"
			>
				<div class="flex items-center justify-between gap-4 px-4 py-3">
					<dt class="text-sm text-muted">{t('inv.role')}</dt>
					<dd class="text-sm font-medium">{roleDisplayName(selected.role_name)}</dd>
				</div>
				<div class="flex items-center justify-between gap-4 px-4 py-3">
					<dt class="text-sm text-muted">{t('inv.detail.invitedBy')}</dt>
					<dd class="truncate text-sm font-medium">{selected.invited_by}</dd>
				</div>
				<div class="flex items-center justify-between gap-4 px-4 py-3">
					<dt class="text-sm text-muted">{t('inv.detail.expires')}</dt>
					<dd class="text-sm font-medium {exp.urgent ? 'text-warning' : ''}">{exp.label}</dd>
				</div>
			</dl>

			{#if detailMessage}
				<div class="mt-4"><Alert align="start">{detailMessage}</Alert></div>
			{/if}

			<div class="mt-6 flex justify-end gap-2">
				<form method="POST" action="?/reject" use:enhance={submitAction('reject')}>
					<input type="hidden" name="id" value={selected.id} />
					<Button
						type="submit"
						variant="ghost"
						loading={acting === 'reject'}
						disabled={acting === 'accept'}
					>
						{acting === 'reject' ? t('inv.reject.submitting') : t('inv.reject')}
					</Button>
				</form>
				<form method="POST" action="?/accept" use:enhance={submitAction('accept')}>
					<input type="hidden" name="id" value={selected.id} />
					<Button type="submit" loading={acting === 'accept'} disabled={acting === 'reject'}>
						{acting === 'accept' ? t('inv.accepting') : t('inv.accept')}
					</Button>
				</form>
			</div>
		{/if}
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('inv.cancel')}></button>
	</form>
</dialog>

<Toaster />
