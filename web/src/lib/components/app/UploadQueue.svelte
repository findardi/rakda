<script lang="ts">
	import { formatBytes } from '$lib/format';
	import { t } from '$lib/i18n';
	import { uploadQueue } from '$lib/upload/queue.svelte';

	const items = $derived(uploadQueue.items);
	const busy = $derived(uploadQueue.busy);
	const total = $derived(items.length);
	const done = $derived(items.filter((i) => i.status === 'done').length);
	const idle = $derived(busy === 0);

	// One shared picker, retargeted per row: a stalled upload needs its File
	// handed back before it can continue, and only a user gesture can supply it.
	let repickInput = $state<HTMLInputElement>();
	let repickId = $state<string | null>(null);

	function askForFile(id: string) {
		repickId = id;
		repickInput?.click();
	}

	function onRepick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (file && repickId) uploadQueue.attach(repickId, file);
		input.value = '';
		repickId = null;
	}
</script>

<input
	bind:this={repickInput}
	onchange={onRepick}
	type="file"
	class="sr-only"
	tabindex="-1"
	aria-hidden="true"
/>

{#if total > 0}
	<section
		class="fixed inset-x-4 bottom-20 z-panel sm:inset-x-auto sm:right-4 sm:bottom-4 sm:w-96"
		aria-label={t('doc.upload.title')}
	>
		<div class="overflow-hidden rounded-box border border-base-content/12 bg-base-100 shadow-lg">
			<header class="flex items-center gap-2 border-b border-base-content/8 px-3 py-2">
				<h2 aria-live="polite" class="min-w-0 flex-1 truncate text-sm font-medium">
					{#if busy > 0}
						{t('doc.upload.uploading', { n: busy })}
					{:else if uploadQueue.failed > 0}
						{t('doc.upload.failed', { n: uploadQueue.failed })}
					{:else if uploadQueue.stalled > 0}
						{t('doc.upload.stalledCount', { n: uploadQueue.stalled })}
					{:else}
						{t('doc.upload.allDone', { n: done })}
					{/if}
				</h2>

				{#if idle}
					<button
						type="button"
						onclick={() => uploadQueue.clearFinished()}
						class="rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
					>
						{t('doc.upload.clear')}
					</button>
				{/if}

				<button
					type="button"
					onclick={() => (uploadQueue.open = !uploadQueue.open)}
					aria-expanded={uploadQueue.open}
					aria-label={uploadQueue.open ? t('doc.collapse') : t('doc.expand')}
					class="grid h-6 w-6 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
				>
					<svg
						class="rakda-chevron h-3.5 w-3.5 {uploadQueue.open ? '' : 'rotate-180'}"
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

			{#if uploadQueue.storageBlocked}
				<p class="border-b border-base-content/8 px-3 py-2 text-xs text-warning text-pretty">
					{t('doc.upload.err.noStorage')}
				</p>
			{/if}

			{#if uploadQueue.open}
				<ul class="max-h-72 divide-y divide-base-content/6 overflow-y-auto">
					{#each items as item (item.id)}
						<li class="px-3 py-2">
							<div class="flex items-baseline gap-2">
								<span class="min-w-0 flex-1 truncate text-sm" title={item.name}>{item.name}</span>
								<span class="flex-none font-mono text-[0.6875rem] text-muted tabular-nums">
									{formatBytes(item.size)}
								</span>
							</div>

							<div class="mt-1 flex items-center gap-2">
								<span class="min-w-0 flex-1 truncate text-xs text-muted">
									{item.status === 'error'
										? (item.message ?? t('err.generic'))
										: item.status === 'canceled'
											? t('doc.upload.status.canceled')
											: item.status === 'stalled'
												? (item.message ?? t('doc.upload.status.stalled'))
												: item.folderName}
								</span>

								{#if item.status === 'uploading' || item.status === 'pending'}
									<button
										type="button"
										onclick={() => uploadQueue.cancel(item.id)}
										class="flex-none rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
									>
										{t('doc.cancel')}
									</button>
								{:else if item.status === 'stalled'}
									<button
										type="button"
										onclick={() => askForFile(item.id)}
										aria-label={t('doc.upload.repickOf', { name: item.name })}
										class="flex-none rounded-field px-1.5 py-0.5 text-xs font-medium text-primary transition-colors hover:bg-primary/8"
									>
										{t('doc.upload.repick')}
									</button>
								{:else if item.status === 'error' && !item.terminal}
									<button
										type="button"
										onclick={() => uploadQueue.retry(item.id)}
										class="flex-none rounded-field px-1.5 py-0.5 text-xs font-medium text-primary transition-colors hover:bg-primary/8"
									>
										{t('doc.upload.retry')}
									</button>
								{:else}
									<button
										type="button"
										onclick={() => uploadQueue.remove(item.id)}
										aria-label={t('doc.upload.remove', { name: item.name })}
										class="grid h-5 w-5 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
									>
										<svg
											class="h-3.5 w-3.5"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="2"
											stroke-linecap="round"
											stroke-linejoin="round"
											aria-hidden="true"
										>
											<path d="M18 6 6 18M6 6l12 12" />
										</svg>
									</button>
								{/if}
							</div>

							{#if item.status === 'uploading' || item.status === 'pending'}
								<div
									class="mt-1.5 h-0.5 overflow-hidden rounded-full bg-base-content/8"
									role="progressbar"
									aria-valuenow={item.progress}
									aria-valuemin={0}
									aria-valuemax={100}
									aria-label={t('doc.upload.progressOf', { name: item.name })}
								>
									<div
										class="rakda-bar h-full w-full origin-left bg-primary"
										style="transform: scaleX({item.progress / 100})"
									></div>
								</div>
							{:else if item.status === 'done'}
								<div class="mt-1.5 h-0.5 rounded-full bg-success/60"></div>
							{:else if item.status === 'error'}
								<div class="mt-1.5 h-0.5 rounded-full bg-error/60"></div>
							{:else if item.status === 'stalled'}
								<div class="mt-1.5 h-0.5 rounded-full bg-warning/60"></div>
							{:else}
								<div class="mt-1.5 h-0.5 rounded-full bg-base-content/10"></div>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</section>
{/if}

<style>
	.rakda-bar {
		transition: transform 200ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.rakda-chevron {
		transition: transform 200ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-bar,
		.rakda-chevron {
			transition: none;
		}
	}
</style>
