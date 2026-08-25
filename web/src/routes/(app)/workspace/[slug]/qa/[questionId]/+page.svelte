<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, TextareaField, showToast } from '$lib/components/common';
	import { roleDisplayName } from '$lib/access/permissions';
	import { formatDateTime } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { QuestionStatus } from '$lib/types/qa';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const slug = $derived(page.params.slug ?? '');
	const basePath = $derived(resolve('/(app)/workspace/[slug]/qa', { slug }));
	const thread = $derived(data.thread);

	const statusMeta = (s: QuestionStatus) => {
		if (s === 'waiting') return { label: t('qa.status.waiting'), dot: 'bg-warning' };
		if (s === 'answered') return { label: t('qa.status.answered'), dot: 'bg-success' };
		return { label: t('qa.status.closed'), dot: 'bg-base-content/40' };
	};
	const status = $derived(statusMeta(thread.status));

	const isAsker = $derived(thread.author_id === data.user?.id);
	const canClose = $derived(thread.status !== 'closed' && (data.isManager || isAsker));
	const canReopen = $derived(data.isManager && thread.status === 'closed');
	const canPromote = $derived(data.isManager && thread.status !== 'waiting');
	const canReply = $derived(thread.status !== 'closed' && (data.isManager || data.qa.qa_enabled));

	const docHref = $derived(
		thread.document_ref?.folder_id
			? resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
					slug,
					folderId: thread.document_ref.folder_id,
					documentId: thread.document_ref.id
				})
			: ''
	);
	const folderHref = $derived(
		thread.folder_ref
			? resolve('/(app)/workspace/[slug]/document/[folderId]', {
					slug,
					folderId: thread.folder_ref.id
				})
			: ''
	);

	let replyBody = $state('');
	let replySubmitting = $state(false);
	let replyMessage = $state<string | null>(null);

	const submitReply: SubmitFunction = () => {
		replySubmitting = true;
		return async ({ result }) => {
			replySubmitting = false;
			if (result.type === 'success') {
				replyBody = '';
				replyMessage = null;
				await invalidateAll();
				showToast(t('qa.thread.replied'), 'success');
			} else if (result.type === 'failure') {
				replyMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				replyMessage = t('err.generic');
			}
		};
	};

	let statusSubmitting = $state(false);
	// Close is irreversible for the asker (reopen is manager-only), so guests
	// confirm first; managers close in one click since they can reopen.
	let confirmingClose = $state(false);

	const submitStatus =
		(toast: string): SubmitFunction =>
		() => {
			statusSubmitting = true;
			return async ({ result }) => {
				statusSubmitting = false;
				if (result.type === 'success') {
					await invalidateAll();
					showToast(toast, 'success');
				} else if (result.type === 'failure') {
					showToast((result.data?.message as string) ?? t('err.generic'), 'error');
				} else {
					showToast(t('err.generic'), 'error');
				}
			};
		};

	let promoteDialog = $state<HTMLDialogElement>();
	let promoteQuestion = $state('');
	let promoteAnswer = $state('');
	let promoteMessage = $state<string | null>(null);
	let promoteSubmitting = $state(false);

	function openPromote() {
		promoteQuestion = thread.subject;
		const lastManagerReply = [...thread.replies]
			.reverse()
			.find((r) => r.author_role === 'owner' || r.author_role === 'admin');
		promoteAnswer = lastManagerReply?.body ?? '';
		promoteMessage = null;
		promoteDialog?.showModal();
	}

	const submitPromote: SubmitFunction = () => {
		promoteSubmitting = true;
		return async ({ result }) => {
			promoteSubmitting = false;
			if (result.type === 'success') {
				promoteDialog?.close();
				await invalidateAll();
				showToast(t('qa.faq.published'), 'success');
			} else if (result.type === 'failure') {
				promoteMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				promoteMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head>
	<title>#{thread.number} {thread.subject} · {t('qa.title')} · {t('brand.name')}</title>
</svelte:head>

<div class="mt-6">
	<a
		href={basePath}
		class="inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-base-content"
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
		{t('qa.thread.back')}
	</a>

	<div class="mt-4 flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0">
			<h2 class="text-xl font-semibold tracking-[-0.01em] text-pretty">{thread.subject}</h2>
			<p class="mt-1 font-mono text-xs text-muted tabular-nums">
				#{thread.number}{data.isManager ? ` · ${thread.group_name}` : ''} · {formatDateTime(
					thread.created_at
				)}
			</p>
		</div>
		<div class="flex flex-none items-center gap-2">
			<span class="inline-flex items-center gap-1.5" title={status.label} aria-live="polite">
				<span class="h-1.5 w-1.5 rounded-full {status.dot}" aria-hidden="true"></span>
				<span class="text-xs font-medium">{status.label}</span>
			</span>
			{#if canClose}
				{#if data.isManager}
					<form
						method="POST"
						action="?/close"
						use:enhance={submitStatus(t('qa.thread.closedToast'))}
					>
						<button type="submit" class="btn btn-ghost btn-sm" disabled={statusSubmitting}>
							{t('qa.thread.close')}
						</button>
					</form>
				{:else}
					<button
						type="button"
						class="btn btn-ghost btn-sm"
						onclick={() => (confirmingClose = true)}
						disabled={statusSubmitting || confirmingClose}
					>
						{t('qa.thread.close')}
					</button>
				{/if}
			{/if}
			{#if canReopen}
				<form method="POST" action="?/reopen" use:enhance={submitStatus(t('qa.thread.reopened'))}>
					<button type="submit" class="btn btn-ghost btn-sm" disabled={statusSubmitting}>
						{t('qa.thread.reopen')}
					</button>
				</form>
			{/if}
			{#if canPromote}
				<button type="button" class="btn btn-ghost btn-sm" onclick={openPromote}>
					{t('qa.promote.button')}
				</button>
			{/if}
		</div>
	</div>

	{#if confirmingClose && canClose}
		<div
			class="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-box border border-base-content/10 bg-base-100 px-4 py-3"
		>
			<p class="text-sm text-pretty">{t('qa.thread.closeConfirm')}</p>
			<div class="flex flex-none gap-2">
				<Button type="button" variant="ghost" onclick={() => (confirmingClose = false)}>
					{t('qa.ask.cancel')}
				</Button>
				<form method="POST" action="?/close" use:enhance={submitStatus(t('qa.thread.closedToast'))}>
					<Button type="submit" loading={statusSubmitting}>
						{t('qa.thread.closeConfirmYes')}
					</Button>
				</form>
			</div>
		</div>
	{/if}

	{#if thread.document_ref || thread.folder_ref}
		<div class="mt-3 flex flex-wrap gap-2">
			<!-- eslint-disable svelte/no-navigation-without-resolve -- both hrefs come from resolve(); the rule cannot see through the derived -->
			{#if thread.document_ref}
				<a
					href={docHref}
					class="inline-flex max-w-full items-center gap-1.5 rounded-selector bg-base-content/6 px-2 py-1 text-xs transition-colors hover:bg-base-content/10"
				>
					<svg
						class="h-3.5 w-3.5 flex-none text-muted"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M14 3v4a1 1 0 0 0 1 1h4" />
						<path d="M5 3h9l5 5v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z" />
					</svg>
					<span class="truncate">{thread.document_ref.name}</span>
				</a>
			{/if}
			{#if thread.folder_ref}
				<a
					href={folderHref}
					class="inline-flex max-w-full items-center gap-1.5 rounded-selector bg-base-content/6 px-2 py-1 text-xs transition-colors hover:bg-base-content/10"
				>
					<svg
						class="h-3.5 w-3.5 flex-none text-muted"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path
							d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"
						/>
					</svg>
					<span class="truncate">{thread.folder_ref.name}</span>
				</a>
			{/if}
		</div>
	{/if}

	<div class="mt-4 rounded-box border border-base-content/10 bg-base-100 px-4 py-3.5">
		<div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
			<span class="text-sm font-medium">{thread.author_name}</span>
			<span
				class="rounded-selector bg-base-content/6 px-1.5 py-0.5 align-[0.05em] text-[0.6875rem] text-muted"
				>{roleDisplayName('guest')}</span
			>
		</div>
		<p class="mt-2 text-sm whitespace-pre-line text-pretty">{thread.body}</p>
	</div>

	<h3 class="mt-8 text-sm font-medium text-muted">{t('qa.thread.replies')}</h3>
	{#if thread.replies.length === 0}
		<p class="mt-3 text-sm text-muted">{t('qa.thread.noReplies')}</p>
	{:else}
		<ul class="mt-3 flex flex-col gap-3">
			{#each thread.replies as reply (reply.id)}
				{@const managerReply = reply.author_role === 'owner' || reply.author_role === 'admin'}
				<li
					class="rounded-box border bg-base-100 px-4 py-3.5 {managerReply
						? 'border-primary/25'
						: 'border-base-content/10'}"
				>
					<div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
						<span class="text-sm font-medium">{reply.author_name}</span>
						{#if managerReply}
							<span
								class="rounded-selector bg-primary/10 px-1.5 py-0.5 align-[0.05em] text-[0.6875rem] font-medium text-primary"
								>{t('qa.thread.managerReply')}</span
							>
						{:else}
							<span
								class="rounded-selector bg-base-content/6 px-1.5 py-0.5 align-[0.05em] text-[0.6875rem] text-muted"
								>{roleDisplayName(reply.author_role)}</span
							>
						{/if}
						<span class="ml-auto font-mono text-xs text-muted tabular-nums"
							>{formatDateTime(reply.created_at)}</span
						>
					</div>
					<p class="mt-2 text-sm whitespace-pre-line text-pretty">{reply.body}</p>
				</li>
			{/each}
		</ul>
	{/if}

	{#if thread.status === 'closed'}
		<div class="mt-6 rounded-box border border-base-content/10 bg-base-100 px-4 py-3.5 text-sm">
			<span class="text-muted">{t('qa.thread.closedNote')}</span>
			{#if !data.isManager && data.qa.qa_enabled}
				<a href={basePath} class="ml-1 font-medium text-primary hover:underline"
					>{t('qa.thread.askNew')}</a
				>
			{/if}
		</div>
	{:else if canReply}
		{#if replyMessage}
			<div class="mt-6"><Alert align="start">{replyMessage}</Alert></div>
		{/if}
		<form method="POST" action="?/reply" use:enhance={submitReply} class="mt-6 flex flex-col gap-3">
			<TextareaField
				id="qa-reply-body"
				name="body"
				label={t('qa.thread.replyLabel')}
				placeholder={t('qa.thread.replyPlaceholder')}
				bind:value={replyBody}
				required
				maxlength={5000}
				rows={3}
			/>
			<div class="flex flex-wrap items-center justify-between gap-3">
				<p class="text-xs text-muted">
					{t(data.isManager ? 'qa.thread.replyHint.manager' : 'qa.thread.replyHint.guest')}
				</p>
				<Button type="submit" loading={replySubmitting} disabled={!replyBody.trim()}>
					{replySubmitting ? t('qa.thread.sending') : t('qa.thread.send')}
				</Button>
			</div>
		</form>
	{/if}
</div>

<dialog bind:this={promoteDialog} class="modal" aria-labelledby="qa-promote-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="qa-promote-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('qa.promote.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('qa.promote.note')}</p>

		{#if promoteMessage}
			<div class="mt-4"><Alert align="start">{promoteMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/promote"
			use:enhance={submitPromote}
			class="mt-5 flex flex-col gap-4"
		>
			<TextareaField
				id="qa-promote-question"
				name="question_text"
				label={t('qa.promote.question')}
				bind:value={promoteQuestion}
				required
				maxlength={150}
				rows={2}
			/>
			<TextareaField
				id="qa-promote-answer"
				name="answer_text"
				label={t('qa.promote.answer')}
				bind:value={promoteAnswer}
				required
				maxlength={5000}
				rows={5}
			/>

			<div class="mt-1 flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => promoteDialog?.close()}>
					{t('qa.ask.cancel')}
				</Button>
				<Button
					type="submit"
					loading={promoteSubmitting}
					disabled={!promoteQuestion.trim() || !promoteAnswer.trim()}
				>
					{promoteSubmitting ? t('qa.promote.submitting') : t('qa.promote.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('qa.ask.cancel')}></button>
	</form>
</dialog>
