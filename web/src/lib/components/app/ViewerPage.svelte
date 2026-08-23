<script lang="ts">
	import { t } from '$lib/i18n';
	import type { SearchBox } from '$lib/types/content';

	type Props = {
		pageNumber: number;
		total: number;
		/** Proxy URL for this page's watermarked PNG. */
		src: string;
		/** Report how much of the viewport this page fills (px), for the page marker. */
		onactive?: (page: number, coverage: number) => void;
		/** Hand the wrapper element to the parent for jump/keyboard navigation. */
		onregister?: (page: number, el: HTMLElement | null) => void;
		/** In-document find overlay: normalized 0..1 boxes, coordinates only. */
		boxes?: SearchBox[];
		/** Which box in `boxes` is the navigated-to hit (stronger highlight). */
		activeIndex?: number;
	};

	let {
		pageNumber,
		total,
		src,
		onactive,
		onregister,
		boxes = [],
		activeIndex = undefined
	}: Props = $props();

	let el = $state<HTMLElement>();
	let shouldLoad = $state(false);
	let loaded = $state(false);
	let errored = $state(false);
	let retry = $state(0);

	// no-store already defeats the browser cache; the nonce only forces the <img>
	// to re-request after a failed load (same src string would be a no-op).
	const resolvedSrc = $derived(shouldLoad ? (retry > 0 ? `${src}&_r=${retry}` : src) : undefined);

	function reload() {
		errored = false;
		loaded = false;
		retry += 1;
	}

	$effect(() => {
		const node = el;
		if (!node) return;

		onregister?.(pageNumber, node);

		// Prefetch a screen ahead so a page is ready by the time it scrolls in.
		const loader = new IntersectionObserver(
			(entries) => {
				for (const e of entries) {
					if (e.isIntersecting) {
						shouldLoad = true;
						loader.disconnect();
					}
				}
			},
			{ rootMargin: '800px 0px' }
		);
		loader.observe(node);

		// Track on-screen coverage so the toolbar can name the page in view, and so
		// the dwell beacon knows which page owns the screen. Height of the visible
		// slice (not target ratio) keeps tall and short pages comparable.
		const spy = new IntersectionObserver(
			(entries) => {
				for (const e of entries) {
					onactive?.(pageNumber, e.isIntersecting ? e.intersectionRect.height : 0);
				}
			},
			{ threshold: Array.from({ length: 21 }, (_, i) => i / 20) }
		);
		spy.observe(node);

		return () => {
			loader.disconnect();
			spy.disconnect();
			// A disconnected observer reports nothing, so say it here: a version
			// switch remounts every page, and stale coverage would keep the dwell
			// clock running on a page that is no longer on screen.
			onactive?.(pageNumber, 0);
			onregister?.(pageNumber, null);
		};
	});
</script>

<div
	bind:this={el}
	data-page={pageNumber}
	role="presentation"
	oncontextmenu={(e) => e.preventDefault()}
	class="rakda-vp relative mx-auto w-full overflow-hidden rounded-box border border-base-content/10 bg-base-100"
	style={loaded ? undefined : 'aspect-ratio: 1 / 1.414;'}
>
	<div class="pointer-events-none absolute left-2 top-2 z-10">
		<span
			class="rounded-selector bg-base-content/90 px-1.5 py-0.5 font-mono text-[0.6875rem] leading-none text-base-100 tabular-nums"
		>
			{pageNumber}<span class="opacity-60">/{total}</span>
		</span>
	</div>

	{#if resolvedSrc}
		<img
			src={resolvedSrc}
			alt={t('doc.view.pageOf', { n: pageNumber, total })}
			draggable="false"
			decoding="async"
			onload={() => {
				loaded = true;
				errored = false;
			}}
			onerror={() => {
				errored = true;
			}}
			class="rakda-vp-img block h-auto w-full select-none {loaded ? 'is-loaded' : 'opacity-0'}"
		/>
	{/if}

	<!-- In-document find overlay: coordinates only, never text (9-f). The
	     page is a <img>, so boxes sit in percentage space and stay put at any
	     render DPI. -->
	{#if boxes.length > 0}
		<div class="pointer-events-none absolute inset-0" aria-hidden="true">
			{#each boxes as b, i (i)}
				<span
					class="absolute border {i === activeIndex
						? 'border-primary bg-primary/25'
						: 'border-primary/70 bg-primary/10'}"
					style="left:{b.x * 100}%; top:{b.y * 100}%; width:{b.w * 100}%; height:{b.h * 100}%;"
				></span>
			{/each}
		</div>
	{/if}

	{#if errored}
		<div
			class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-base-100 px-6 text-center"
		>
			<svg
				class="h-7 w-7 text-muted/70"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<path
					d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"
				/>
				<path d="M12 9v4M12 17h.01" />
			</svg>
			<p class="text-sm text-muted">{t('doc.view.pageError')}</p>
			<button
				type="button"
				onclick={reload}
				class="rounded-field px-2.5 py-1 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
			>
				{t('doc.view.retry')}
			</button>
		</div>
	{:else if !loaded}
		<div class="rakda-vp-skel absolute inset-0" aria-hidden="true"></div>
	{/if}
</div>

<style>
	.rakda-vp-skel {
		background-color: color-mix(in oklch, var(--color-base-content) 6%, transparent);
		animation: rakda-vp-pulse 1400ms ease-in-out infinite;
	}
	@keyframes rakda-vp-pulse {
		50% {
			opacity: 0.5;
		}
	}
	.rakda-vp-img {
		-webkit-touch-callout: none;
		transition: opacity 220ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.rakda-vp-img.is-loaded {
		opacity: 1;
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-vp-skel {
			animation: none;
		}
		.rakda-vp-img {
			transition: none;
		}
	}
</style>
