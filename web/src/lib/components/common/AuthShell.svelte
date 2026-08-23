<script lang="ts">
	import type { Snippet } from 'svelte';
	import Brand from './Brand.svelte';
	import LanguageSwitcher from './LanguageSwitcher.svelte';
	import { t } from '$lib/i18n';

	let { children }: { children: Snippet } = $props();
</script>

<div
	class="grid min-h-dvh grid-rows-[auto_1fr] min-[880px]:grid-cols-[minmax(340px,38%)_1fr] min-[880px]:grid-rows-1"
>
	<aside
		class="flex flex-col gap-8 border-b border-base-content/10 bg-base-300 px-6 py-5 min-[880px]:justify-between min-[880px]:border-r min-[880px]:border-b-0 min-[880px]:p-9"
	>
		<div class="flex items-center justify-between gap-3">
			<a href="/" class="inline-flex w-fit"><Brand size={30} /></a>
			<LanguageSwitcher />
		</div>

		<div class="hidden min-[880px]:block">
			<p class="max-w-[22ch] text-2xl font-medium tracking-[-0.015em] text-balance">
				{t('brand.tagline')}
			</p>
			<div class="mt-6 max-w-[34ch] border-t border-base-content/10 pt-6">
				<h2 class="text-[0.9375rem] font-semibold">{t('brand.reassure.title')}</h2>
				<p class="mt-1.5 text-sm leading-relaxed text-muted">{t('brand.reassure.body')}</p>
			</div>
		</div>

		<p class="hidden text-sm text-muted min-[880px]:block">© Rakda</p>
	</aside>

	<main class="flex justify-center px-5 pt-10 pb-14 min-[880px]:items-center min-[880px]:p-8">
		<div class="w-full max-w-100">
			{@render children()}
		</div>
	</main>
</div>
