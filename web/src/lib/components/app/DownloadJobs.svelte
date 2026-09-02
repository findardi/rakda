<script lang="ts">
	import { downloadJobs } from '$lib/download/jobs.svelte';
	import { formatBytes } from '$lib/format';
	import { t } from '$lib/i18n';

	const jobs = $derived(downloadJobs.visible);
	const pending = $derived(downloadJobs.pending);
</script>

{#if jobs.length > 0}
	<section
		class="fixed inset-x-4 bottom-20 z-panel sm:inset-x-auto sm:right-4 sm:bottom-4 sm:w-96"
		aria-label={t('doc.dl.title')}
	>
		<div class="overflow-hidden rounded-box border border-base-content/12 bg-base-100 shadow-lg">
			<header class="flex items-center gap-2 border-b border-base-content/8 px-3 py-2">
				<h2 aria-live="polite" class="min-w-0 flex-1 truncate text-sm font-medium">
					{#if pending > 0}
						{t('doc.dl.preparing', { n: pending })}
					{:else}
						{t('doc.dl.ready', { n: jobs.length })}
					{/if}
				</h2>

				<button
					type="button"
					onclick={() => (downloadJobs.panelOpen = !downloadJobs.panelOpen)}
					aria-expanded={downloadJobs.panelOpen}
					aria-label={downloadJobs.panelOpen ? t('doc.collapse') : t('doc.expand')}
					class="grid h-6 w-6 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
				>
					<svg
						class="rakda-chevron h-3.5 w-3.5 {downloadJobs.panelOpen ? '' : 'rotate-180'}"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="m6 9 6 6 6-6" />
					</svg>
				</button>
			</header>

			{#if downloadJobs.panelOpen}
				<ul class="max-h-72 divide-y divide-base-content/8 overflow-y-auto">
					{#each jobs as job (job.id)}
						<li class="flex items-center gap-3 px-3 py-2">
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm">{job.document_name}</p>
								<p class="font-mono text-xs text-muted">
									{#if job.status === 'pending'}
										{t('doc.dl.pages', { n: job.page_count })}
									{:else if job.status === 'ready'}
										{formatBytes(job.size_bytes)}
									{:else}
										{job.error || t('err.generic')}
									{/if}
								</p>
							</div>

							{#if job.status === 'pending'}
								<span class="loading loading-xs loading-spinner text-muted" aria-hidden="true"
								></span>
							{:else if job.status === 'ready'}
								<!-- eslint-disable svelte/no-navigation-without-resolve -- endpoint, not a page: resolve() has no entry for /api routes -->
								<a
									href={downloadJobs.href(job)}
									download={job.file_name}
									onclick={() => downloadJobs.dismiss(job.id)}
									class="btn btn-xs btn-primary"
								>
									{t('doc.dl.save')}
								</a>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							{:else}
								<button
									type="button"
									onclick={() => downloadJobs.dismiss(job.id)}
									class="rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
								>
									{t('doc.dl.dismiss')}
								</button>
							{/if}
						</li>
					{/each}
				</ul>

				<p class="border-t border-base-content/8 px-3 py-2 text-xs text-muted">
					{t('doc.dl.expiryHint')}
				</p>
			{/if}
		</div>
	</section>
{/if}
