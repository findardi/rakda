<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, TextareaField, showToast } from '$lib/components/common';
	import { formatDate } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let faqDialog = $state<HTMLDialogElement>();
	let questionText = $state('');
	let answerText = $state('');
	let faqMessage = $state<string | null>(null);
	let faqSubmitting = $state(false);

	function openWrite() {
		questionText = '';
		answerText = '';
		faqMessage = null;
		faqDialog?.showModal();
	}

	const submitFaq: SubmitFunction = () => {
		faqSubmitting = true;
		return async ({ result }) => {
			faqSubmitting = false;
			if (result.type === 'success') {
				faqDialog?.close();
				await invalidateAll();
				showToast(t('qa.faq.published'), 'success');
			} else if (result.type === 'failure') {
				faqMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				faqMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head>
	<title>{t('qa.tab.faq')} · {t('qa.title')} · {t('brand.name')}</title>
</svelte:head>

{#if data.isManager}
	<div class="mt-6 flex justify-end">
		<button type="button" class="btn btn-primary btn-sm" onclick={openWrite}>
			{t('qa.faq.write')}
		</button>
	</div>
{/if}

{#if data.faqs.length === 0}
	<div
		class="mt-6 flex flex-col items-center justify-center gap-3 rounded-box border border-base-content/10 bg-base-100 px-6 py-16 text-center"
	>
		<svg
			class="h-9 w-9 text-muted/70"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.4"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M21 12a8 8 0 0 1-8 8H5a2 2 0 0 1-2-2v-6a8 8 0 1 1 18 0z" />
			<path
				d="M9.5 10.5c0-1.4 1.1-2.5 2.5-2.5s2.5 1.1 2.5 2.5c0 1.2-.9 1.8-1.7 2.3-.5.3-.8.7-.8 1.2"
			/>
			<path d="M12 16.5h.01" />
		</svg>
		<div>
			<p class="text-[0.9375rem] font-medium">{t('qa.faq.empty.title')}</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">{t('qa.faq.empty.body')}</p>
		</div>
	</div>
{:else}
	<ul class="mt-6 divide-y divide-base-content/8 border-t border-base-content/8">
		{#each data.faqs as faq (faq.id)}
			<li class="py-4">
				<p class="text-[0.9375rem] font-medium text-pretty">{faq.question_text}</p>
				<p class="mt-1.5 text-sm whitespace-pre-line text-pretty">{faq.answer_text}</p>
				<p class="mt-2 font-mono text-xs text-muted tabular-nums">{formatDate(faq.created_at)}</p>
			</li>
		{/each}
	</ul>
{/if}

<dialog bind:this={faqDialog} class="modal" aria-labelledby="qa-faq-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="qa-faq-title" class="text-lg font-semibold tracking-[-0.01em]">{t('qa.faq.write')}</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('qa.promote.note')}</p>

		{#if faqMessage}
			<div class="mt-4"><Alert align="start">{faqMessage}</Alert></div>
		{/if}

		<form method="POST" action="?/faq" use:enhance={submitFaq} class="mt-5 flex flex-col gap-4">
			<TextareaField
				id="qa-faq-question"
				name="question_text"
				label={t('qa.promote.question')}
				bind:value={questionText}
				required
				maxlength={150}
				rows={2}
			/>
			<TextareaField
				id="qa-faq-answer"
				name="answer_text"
				label={t('qa.promote.answer')}
				bind:value={answerText}
				required
				maxlength={5000}
				rows={5}
			/>

			<div class="mt-1 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => faqDialog?.close()}>
					{t('qa.ask.cancel')}
				</Button>
				<Button
					type="submit"
					loading={faqSubmitting}
					disabled={!questionText.trim() || !answerText.trim()}
				>
					{faqSubmitting ? t('qa.promote.submitting') : t('qa.promote.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('qa.ask.cancel')}></button>
	</form>
</dialog>
