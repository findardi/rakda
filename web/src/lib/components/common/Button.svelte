<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		type?: 'button' | 'submit';
		variant?: 'primary' | 'ghost' | 'danger' | 'danger-outline';
		size?: 'sm' | 'md';
		loading?: boolean;
		disabled?: boolean;
		full?: boolean;
		onclick?: () => void;
		children: Snippet;
	};

	let {
		type = 'button',
		variant = 'primary',
		size = 'md',
		loading = false,
		disabled = false,
		full = false,
		onclick,
		children
	}: Props = $props();

	const variantClass = $derived(
		variant === 'primary'
			? 'btn-primary'
			: variant === 'danger'
				? 'btn-error'
				: variant === 'danger-outline'
					? 'btn-outline btn-error'
					: 'btn-ghost'
	);
</script>

<button
	{type}
	{onclick}
	class="btn {variantClass}"
	class:btn-sm={size === 'sm'}
	class:btn-block={full}
	disabled={disabled || loading}
	aria-busy={loading}
>
	{#if loading}<span
			class="loading loading-spinner"
			class:loading-xs={size === 'sm'}
			class:loading-sm={size !== 'sm'}
		></span>{/if}
	{@render children()}
</button>
