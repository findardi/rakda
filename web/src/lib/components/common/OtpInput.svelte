<script lang="ts">
	import { t } from '$lib/i18n';

	type Props = {
		value?: string;
		invalid?: boolean;
		autofocus?: boolean;
	};
	let { value = $bindable(''), invalid = false, autofocus = false }: Props = $props();

	let el = $state<HTMLInputElement>();

	$effect(() => {
		if (autofocus) el?.focus();
	});

	// One field, not six boxes: the browser's one-time-code autofill and a
	// pasted code both land in it whole; digits only, six at most.
	function onInput(e: Event) {
		const input = e.target as HTMLInputElement;
		value = input.value.replace(/\D/g, '').slice(0, 6);
		input.value = value;
	}
</script>

<input
	bind:this={el}
	{value}
	type="text"
	inputmode="numeric"
	autocomplete="one-time-code"
	maxlength="6"
	pattern="[0-9]{6}"
	aria-label={t('otp.group')}
	class="input w-48 text-center font-mono text-2xl tracking-[0.5em]"
	class:input-error={invalid}
	oninput={onInput}
/>
