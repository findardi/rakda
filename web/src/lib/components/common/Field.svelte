<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';

	type Props = {
		id: string;
		name: string;
		label: string;
		type?: HTMLInputAttributes['type'];
		value?: string;
		error?: string;
		hint?: string;
		placeholder?: string;
		autocomplete?: HTMLInputAttributes['autocomplete'];
		inputmode?: HTMLInputAttributes['inputmode'];
		required?: boolean;
		maxlength?: number;
		autofocus?: boolean;
	};

	let {
		id,
		name,
		label,
		type = 'text',
		value = $bindable(''),
		error,
		hint,
		placeholder,
		autocomplete,
		inputmode,
		required = false,
		maxlength,
		autofocus = false
	}: Props = $props();

	let inputEl = $state<HTMLInputElement>();
	// Focus on mount when requested (step changes, primary field).
	$effect(() => {
		if (autofocus) inputEl?.focus();
	});

	const describedBy = $derived(
		[error ? `${id}-error` : null, hint && !error ? `${id}-hint` : null]
			.filter(Boolean)
			.join(' ') || undefined
	);
</script>

<div class="flex flex-col gap-1.5">
	<label class="text-sm font-medium" for={id}>{label}</label>
	<input
		bind:this={inputEl}
		{id}
		{name}
		{type}
		{placeholder}
		{required}
		{maxlength}
		{autocomplete}
		{inputmode}
		bind:value
		class="input w-full text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
		class:input-error={!!error}
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={describedBy}
	/>
	{#if hint && !error}<p class="text-sm text-muted" id="{id}-hint">{hint}</p>{/if}
	{#if error}<p class="text-sm text-error" id="{id}-error">{error}</p>{/if}
</div>
