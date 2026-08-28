<script lang="ts">
	import { tick } from 'svelte';
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { assignableRoles } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { AddMemberResult, WorkspaceRoleData } from '$lib/types/workspace';

	type Props = {
		roles: WorkspaceRoleData[];
		/** The inviter's own role — limits which roles they may grant. */
		viewerRole: string;
		/** Form action URL — same-route (`?/invite`) or a cross-route path. */
		action: string;
		/** Controls visibility; the host opens by setting this true. */
		open?: boolean;
		/** When set, the result view links here to review pending invitations. */
		pendingHref?: string;
		/** Fired after a successful submit + reload, with the count newly invited. */
		oncompleted?: (invited: number) => void;
	};

	let {
		roles,
		viewerRole,
		action,
		open = $bindable(false),
		pendingHref,
		oncompleted
	}: Props = $props();

	// Only the roles the inviter is allowed to grant (owner → all but owner;
	// admin → guest only). Backend enforces the same; this just hides the rest.
	const roleOptions = $derived(assignableRoles(viewerRole, roles));
	// Default to the least-privileged role so a careless batch can't over-grant.
	const defaultRoleId = $derived(
		roleOptions.find((r) => r.name === 'guest')?.id ?? roleOptions.at(-1)?.id ?? ''
	);

	const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
	const MAX_EMAILS = 50;

	let dialog = $state<HTMLDialogElement>();
	let emails = $state<string[]>([]);
	let draft = $state('');
	let chipError = $state<string | null>(null);
	let roleId = $state('');
	let submitting = $state(false);
	let formError = $state<string | null>(null);
	let results = $state<AddMemberResult[] | null>(null);

	const summary = $derived.by(() => {
		const r = results ?? [];
		return {
			invited: r.filter((x) => x.outcome === 'invited').length,
			alreadyMember: r.filter((x) => x.reason === 'already_member').length,
			alreadyInvited: r.filter((x) => x.reason === 'already_invited').length
		};
	});

	function reset() {
		emails = [];
		draft = '';
		chipError = null;
		formError = null;
		results = null;
		roleId = defaultRoleId;
	}

	function focusInput() {
		tick().then(() => dialog?.querySelector<HTMLInputElement>('#invite-email')?.focus());
	}

	// Opening is driven by the bindable `open`; closing flows back through onclose.
	$effect(() => {
		if (open) {
			reset();
			dialog?.showModal();
			focusInput();
		}
	});

	function inviteMore() {
		reset();
		focusInput();
	}

	function addEmail(raw: string): boolean {
		const email = raw.trim().toLowerCase();
		if (!email) return false;
		if (!EMAIL_RE.test(email)) {
			chipError = t('member.invite.invalidEmail');
			return false;
		}
		if (emails.includes(email)) {
			chipError = t('member.invite.dupe');
			return false;
		}
		if (emails.length >= MAX_EMAILS) {
			chipError = t('member.invite.max');
			return false;
		}
		emails = [...emails, email];
		chipError = null;
		return true;
	}

	function commitDraft() {
		if (draft.trim() && addEmail(draft)) draft = '';
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			commitDraft();
		} else if (e.key === 'Backspace' && draft === '' && emails.length) {
			emails = emails.slice(0, -1);
		}
	}

	function onPaste(e: ClipboardEvent) {
		const text = e.clipboardData?.getData('text') ?? '';
		// Let a single clean email fall through to the input; split only on lists.
		if (!/[,\s;]/.test(text)) return;
		e.preventDefault();
		for (const part of text.split(/[,\s;]+/)) addEmail(part);
		draft = '';
	}

	function removeEmail(email: string) {
		emails = emails.filter((x) => x !== email);
		chipError = null;
	}

	const submit: SubmitFunction = ({ formData, cancel }) => {
		commitDraft(); // fold a typed-but-unadded email into the batch
		if (!emails.length) {
			chipError = t('member.invite.empty');
			cancel();
			return;
		}
		// Rebuild from state so the just-committed chip is included.
		formData.delete('email');
		for (const e of emails) formData.append('email', e);
		formData.set('roleId', roleId);

		submitting = true;
		formError = null;
		return async ({ result }) => {
			submitting = false;
			if (result.type === 'success') {
				const d = result.data as { results?: AddMemberResult[] } | undefined;
				results = d?.results ?? [];
				await invalidateAll();
				oncompleted?.(results.filter((r) => r.outcome === 'invited').length);
			} else if (result.type === 'failure') {
				const d = result.data as
					| { fieldErrors?: Record<string, string>; message?: string }
					| undefined;
				formError = d?.fieldErrors?.email ?? d?.message ?? t('err.generic');
			} else {
				formError = t('err.generic');
			}
		};
	};
</script>

<dialog
	bind:this={dialog}
	onclose={() => (open = false)}
	class="modal"
	aria-labelledby="invite-dialog-title"
>
	<div class="modal-box max-w-lg rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="invite-dialog-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('member.invite.title')}
		</h2>

		{#if results}
			<!-- Result: outcome only; never which emails were already registered. -->
			{@const ok = summary.invited > 0}
			<div
				class="mt-4 flex items-center gap-3 rounded-box border border-base-content/10 p-4"
				aria-live="polite"
			>
				<span
					class="grid h-10 w-10 flex-none place-items-center rounded-full {ok
						? 'bg-success/10 text-success'
						: 'bg-base-content/5 text-muted'}"
					aria-hidden="true"
				>
					<svg
						class="h-5 w-5"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<rect x="3" y="5" width="18" height="14" rx="2" />
						<path d="m3 7 9 6 9-6" />
						{#if ok}<path d="m16 13 2 2 4-4" />{/if}
					</svg>
				</span>
				<div class="min-w-0 flex-1">
					<p class="text-sm font-semibold">
						{ok
							? t('member.invite.result.invited', { n: summary.invited })
							: t('member.invite.result.none')}
					</p>
					{#if summary.alreadyMember || summary.alreadyInvited}
						<p class="mt-0.5 text-xs text-muted">
							{#if summary.alreadyMember}{t('member.invite.result.alreadyMember', {
									n: summary.alreadyMember
								})}{/if}{#if summary.alreadyMember && summary.alreadyInvited}
								·
							{/if}{#if summary.alreadyInvited}{t('member.invite.result.alreadyInvited', {
									n: summary.alreadyInvited
								})}{/if}
						</p>
					{/if}
				</div>
			</div>

			<div class="mt-5 flex flex-wrap items-center justify-between gap-2">
				{#if pendingHref}
					<a href={pendingHref} class="text-sm font-medium text-primary hover:underline">
						{t('member.invite.viewPending')}
					</a>
				{:else}
					<span></span>
				{/if}
				<div class="flex gap-2">
					<Button type="button" variant="ghost" onclick={inviteMore}>
						{t('member.invite.more')}
					</Button>
					<Button type="button" onclick={() => dialog?.close()}>
						{t('member.invite.done')}
					</Button>
				</div>
			</div>
		{:else}
			<p class="mt-1 text-sm text-muted text-pretty">{t('member.invite.desc')}</p>

			{#if formError}
				<div class="mt-4"><Alert align="start">{formError}</Alert></div>
			{/if}

			<form method="POST" {action} use:enhance={submit} class="mt-5">
				<div class="flex items-center justify-between gap-2">
					<label class="text-sm font-medium" for="invite-email">
						{t('member.invite.emailLabel')}
					</label>
					<span class="font-mono text-xs text-muted">{emails.length}/{MAX_EMAILS}</span>
				</div>

				<!-- Chips field: removable email chips followed by a free-text input. -->
				<div
					class="mt-1.5 flex flex-wrap items-center gap-1.5 rounded-field border border-base-content/15 bg-base-100 p-2 transition-colors focus-within:border-primary"
					class:border-error={!!chipError}
				>
					{#each emails as email (email)}
						<span
							class="inline-flex items-center gap-1 rounded-selector bg-base-content/6 py-0.5 pr-0.5 pl-2 font-mono text-sm"
						>
							{email}
							<button
								type="button"
								onclick={() => removeEmail(email)}
								aria-label={t('member.invite.removeChip', { email })}
								class="grid h-6 w-6 place-items-center rounded-selector text-muted transition-colors hover:bg-base-content/10 hover:text-base-content"
							>
								<svg
									class="h-3.5 w-3.5"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									aria-hidden="true"
								>
									<path d="M18 6 6 18M6 6l12 12" />
								</svg>
							</button>
						</span>
					{/each}
					<input
						id="invite-email"
						type="email"
						bind:value={draft}
						onkeydown={onKeydown}
						onpaste={onPaste}
						onblur={commitDraft}
						placeholder={emails.length ? '' : t('member.invite.emailPlaceholder')}
						autocomplete="off"
						inputmode="email"
						aria-describedby="invite-email-hint"
						class="min-w-[14ch] flex-1 bg-transparent px-1 py-0.5 text-sm outline-none placeholder:text-muted"
					/>
				</div>

				{#if chipError}
					<p class="mt-1.5 text-sm text-error">{chipError}</p>
				{:else}
					<p id="invite-email-hint" class="mt-1.5 text-xs text-muted">
						{t('member.invite.emailHint')}
					</p>
				{/if}

				<div class="mt-4">
					<label class="text-sm font-medium" for="invite-role">{t('member.invite.roleLabel')}</label
					>
					<select id="invite-role" name="roleId" bind:value={roleId} class="select mt-1.5 w-full">
						{#each roleOptions as r (r.id)}
							<option value={r.id}>{roleDisplayName(r.name)}</option>
						{/each}
					</select>
				</div>

				<div class="mt-6 flex flex-wrap justify-end gap-2">
					<Button type="button" variant="ghost" onclick={() => dialog?.close()}>
						{t('member.invite.close')}
					</Button>
					<Button type="submit" loading={submitting} disabled={!emails.length && !draft.trim()}>
						{submitting ? t('member.invite.submitting') : t('member.invite.submit')}
					</Button>
				</div>
			</form>
		{/if}
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('member.invite.close')}></button>
	</form>
</dialog>
