<script lang="ts">
	type Props = {
		value?: string;
		invalid?: boolean;
		autofocus?: boolean;
	};
	let { value = $bindable(''), invalid = false, autofocus = false }: Props = $props();

	const LENGTH = 6;
	const cells = Array.from({ length: LENGTH }, (_, i) => i);

	let el = $state<HTMLInputElement>();
	let focused = $state(false);

	$effect(() => {
		if (autofocus) el?.focus();
	});

	// One real field behind six drawn cells: the browser's one-time-code
	// autofill and a pasted code both land in it whole; the cells only mirror
	// its value. A full code submits its form — the button stays as fallback.
	function onInput(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		value = input.value.replace(/\D/g, '').slice(0, LENGTH);
		input.value = value;
		if (value.length === LENGTH) input.form?.requestSubmit();
	}

	// The caret always sits at the end, where the drawn active cell is.
	function toEnd() {
		el?.setSelectionRange(value.length, value.length);
	}

	// A digit typed into a full code starts a new code with that digit.
	function onKeydown(e: KeyboardEvent) {
		if (value.length === LENGTH && /^\d$/.test(e.key) && el?.selectionStart === el?.selectionEnd) {
			el?.select();
		}
	}

	const active = $derived(Math.min(value.length, LENGTH - 1));
</script>

<div class="relative inline-block">
	<div class="flex gap-1.5 sm:gap-2" aria-hidden="true">
		{#each cells as i (i)}
			<span
				class="rakda-otp-cell grid h-12 w-9 place-items-center rounded-field border bg-base-100 font-mono text-xl tabular-nums transition-colors sm:w-10 {i ===
				3
					? 'ml-1.5 sm:ml-2'
					: ''}"
				class:is-filled={i < value.length}
				class:is-active={focused && i === active}
				class:is-invalid={invalid}
			>
				{#if i < value.length}
					{value[i]}
				{:else if focused && i === value.length}
					<span class="rakda-otp-caret h-6 w-px bg-base-content"></span>
				{/if}
			</span>
		{/each}
	</div>
	<input
		bind:this={el}
		{value}
		id="otp"
		name="code"
		type="text"
		inputmode="numeric"
		autocomplete="one-time-code"
		maxlength="6"
		pattern="[0-9]{6}"
		aria-invalid={invalid || undefined}
		aria-describedby="otp-error"
		class="absolute inset-0 h-full w-full cursor-text opacity-0"
		oninput={onInput}
		onkeydown={onKeydown}
		onmouseup={toEnd}
		onfocus={() => {
			focused = true;
			toEnd();
		}}
		onblur={() => (focused = false)}
	/>
</div>

<style>
	.rakda-otp-cell {
		border-color: color-mix(in oklch, var(--color-base-content) 20%, transparent);
	}
	.rakda-otp-cell.is-filled {
		border-color: color-mix(in oklch, var(--color-base-content) 45%, transparent);
	}
	.rakda-otp-cell.is-active {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--color-primary) 22%, transparent);
	}
	.rakda-otp-cell.is-invalid {
		border-color: var(--color-error);
		color: var(--color-error);
	}
	.rakda-otp-caret {
		animation: rakda-otp-blink 1s steps(2, start) infinite;
	}
	@keyframes rakda-otp-blink {
		to {
			visibility: hidden;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-otp-caret {
			animation: none;
		}
	}
</style>
