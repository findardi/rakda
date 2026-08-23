<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		variant?: 'error' | 'success';
		/** `center` suits standalone auth cards; `start` reads better inside a left-aligned form. */
		align?: 'center' | 'start';
		children: Snippet;
	};
	let { variant = 'error', align = 'center', children }: Props = $props();

	const variantClass = $derived(variant === 'error' ? 'alert-error' : 'alert-success');
	const alignClass = $derived(
		align === 'center' ? 'justify-center text-center' : 'justify-start text-left'
	);
</script>

<div role="alert" class="rakda-alert-in alert alert-soft {variantClass} {alignClass} text-sm">
	{@render children()}
</div>

<style>
	/* State feedback enters with a subtle settle; instant for reduced motion. */
	.rakda-alert-in {
		animation: rakda-alert-in 180ms ease-out;
	}
	@keyframes rakda-alert-in {
		from {
			opacity: 0;
			transform: translateY(-2px);
		}
		to {
			opacity: 1;
			transform: none;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-alert-in {
			animation: none;
		}
	}
</style>
