<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Field, TextareaField, Toaster, showToast } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const slug = $derived(page.params.slug ?? '');
	const basePath = $derived(resolve('/(app)/workspace/[slug]/qa', { slug }));
	const faqPath = $derived(resolve('/(app)/workspace/[slug]/qa/faq', { slug }));

	const subtabs = $derived([
		{ href: basePath, label: t('qa.tab.questions'), count: data.qa.question_count },
		{ href: faqPath, label: t('qa.tab.faq'), count: data.qa.faq_count }
	]);

	// The questions tab covers the index and thread sub-routes, but never /faq.
	const isTabActive = (href: string) => {
		if (href === faqPath) {
			return page.url.pathname === faqPath || page.url.pathname.startsWith(`${faqPath}/`);
		}
		return (
			(page.url.pathname === basePath || page.url.pathname.startsWith(`${basePath}/`)) &&
			!page.url.pathname.startsWith(faqPath)
		);
	};

	const askDisabled = $derived(
		!data.qa.qa_enabled || (data.qa.question_limit != null && (data.qa.quota_remaining ?? 0) <= 0)
	);
	// limit 0 = submissions blocked by design, never "quota used up" — the two
	// states get different sentences per product rule.
	const askDisabledReason = $derived(
		!data.qa.qa_enabled
			? t('qa.limit.disabled')
			: data.qa.question_limit === 0
				? t('qa.limit.blocked')
				: data.qa.question_limit != null && (data.qa.quota_remaining ?? 0) <= 0
					? t('qa.limit.exhausted')
					: ''
	);

	let askDialog = $state<HTMLDialogElement>();
	let subject = $state('');
	let body = $state('');
	let askMessage = $state<string | null>(null);
	let askSubmitting = $state(false);
	let refDoc = $state<{ id: string; name: string } | null>(null);
	let askAutoOpened = false;

	function openAsk(ref?: { id: string; name: string } | null) {
		subject = '';
		body = '';
		askMessage = null;
		refDoc = ref ?? null;
		askDialog?.showModal();
	}

	// Entry points (viewer toolbar, document row) land here with the document
	// reference in the query string; open the dialog once, prefilled.
	$effect(() => {
		if (askAutoOpened) return;
		const docId = page.url.searchParams.get('ask-doc');
		if (!docId) return;
		askAutoOpened = true;
		if (data.isManager || askDisabled) return;
		openAsk({ id: docId, name: page.url.searchParams.get('ask-name') ?? '' });
	});

	const submitAsk: SubmitFunction = () => {
		askSubmitting = true;
		return async ({ result }) => {
			askSubmitting = false;
			if (result.type === 'success') {
				const hadRef = !!refDoc;
				askDialog?.close();
				if (hadRef) {
					await goto(basePath, { invalidateAll: true });
				} else {
					await invalidateAll();
				}
				showToast(t('qa.ask.success'), 'success');
			} else if (result.type === 'failure') {
				askMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				askMessage = t('err.generic');
			}
		};
	};
</script>

<div class="mx-auto w-full max-w-4xl px-6 py-8">
	<header>
		<h1 class="text-2xl font-semibold tracking-[-0.02em]">{t('qa.title')}</h1>
		<p class="mt-1.5 max-w-prose text-sm text-muted">{t('qa.desc')}</p>
	</header>

	<div class="mt-6 flex flex-wrap items-center gap-x-4 gap-y-3">
		<!-- Segmented control: secondary nav, counts fed by the shared layout load. -->
		<nav class="inline-flex gap-1 rounded-field bg-base-content/4 p-1" aria-label={t('qa.title')}>
			{#each subtabs as tab (tab.href)}
				{@const active = isTabActive(tab.href)}
				<a
					href={tab.href}
					aria-current={active ? 'page' : undefined}
					class="inline-flex items-center gap-2 rounded-selector px-3 py-1.5 text-sm font-medium transition-colors {active
						? 'bg-base-100 text-base-content shadow-sm'
						: 'text-muted hover:text-base-content'}"
				>
					{tab.label}
					<span
						class="font-mono text-xs {active ? 'text-primary' : 'text-muted'}"
						aria-hidden="true">{tab.count}</span
					>
				</a>
			{/each}
		</nav>

		{#if !data.isManager}
			<div class="ml-auto flex items-center gap-3">
				{#if askDisabled}
					<span class="text-xs text-muted">{askDisabledReason}</span>
				{:else if data.qa.question_limit != null}
					<span class="font-mono text-xs text-muted tabular-nums"
						>{t('qa.limit.remaining', { n: data.qa.quota_remaining ?? 0 })}</span
					>
				{/if}
				<button
					type="button"
					class="btn btn-primary btn-sm"
					onclick={() => openAsk()}
					disabled={askDisabled}
				>
					{t('qa.ask.button')}
				</button>
			</div>
		{/if}
	</div>

	{@render children()}
</div>

<dialog bind:this={askDialog} class="modal" aria-labelledby="qa-ask-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="qa-ask-title" class="text-lg font-semibold tracking-[-0.01em]">{t('qa.ask.title')}</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('qa.ask.desc')}</p>

		{#if askMessage}
			<div class="mt-4"><Alert align="start">{askMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="{basePath}?/ask"
			use:enhance={submitAsk}
			class="mt-5 flex flex-col gap-4"
		>
			{#if refDoc}
				<div class="flex flex-col gap-1.5">
					<span class="text-sm font-medium">{t('qa.ask.reference')}</span>
					<span
						class="inline-flex w-fit max-w-full items-center gap-1.5 rounded-selector bg-base-content/6 px-2 py-1 text-xs"
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
						<span class="truncate">{refDoc.name}</span>
					</span>
					<input type="hidden" name="document_id" value={refDoc.id} />
				</div>
			{/if}

			<Field
				id="qa-ask-subject"
				name="subject"
				label={t('qa.ask.subject')}
				placeholder={t('qa.ask.subjectPlaceholder')}
				bind:value={subject}
				required
				maxlength={150}
				autofocus
			/>
			<TextareaField
				id="qa-ask-body"
				name="body"
				label={t('qa.ask.body')}
				placeholder={t('qa.ask.bodyPlaceholder')}
				bind:value={body}
				required
				maxlength={5000}
				rows={5}
			/>

			<div class="mt-1 flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => askDialog?.close()}>
					{t('qa.ask.cancel')}
				</Button>
				<Button type="submit" loading={askSubmitting} disabled={!subject.trim() || !body.trim()}>
					{askSubmitting ? t('qa.ask.submitting') : t('qa.ask.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('qa.ask.cancel')}></button>
	</form>
</dialog>

<Toaster />
