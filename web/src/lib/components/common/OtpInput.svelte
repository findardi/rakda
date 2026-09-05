<script lang="ts">
	import { t } from '$lib/i18n';

	type Props = {
		value?: string;
		invalid?: boolean;
		autofocus?: boolean;
	};
	let { value = $bindable(''), invalid = false, autofocus = false }: Props = $props();

	const length = 6;

	let els: HTMLInputElement[] = [];
	let chars = $state<string[]>(Array.from({ length }, () => ''));

	$effect(() => {
		value = chars.join('');
	});

	$effect(() => {
		if (autofocus) els[0]?.focus();
	});

	function onInput(i: number, e: Event) {
		const digits = (e.target as HTMLInputElement).value.replace(/\D/g, '');
		chars[i] = digits ? digits[digits.length - 1] : '';
		if (digits && i < length - 1) els[i + 1]?.focus();
	}

	function onKeydown(i: number, e: KeyboardEvent) {
		if (e.key === 'Backspace' && !chars[i] && i > 0) {
			e.preventDefault();
			chars[i - 1] = '';
			els[i - 1]?.focus();
		} else if (e.key === 'ArrowLeft' && i > 0) {
			els[i - 1]?.focus();
		} else if (e.key === 'ArrowRight' && i < length - 1) {
			els[i + 1]?.focus();
		}
	}

	function onPaste(e: ClipboardEvent) {
		e.preventDefault();
		const digits = (e.clipboardData?.getData('text') ?? '').replace(/\D/g, '').slice(0, length);
		if (!digits) return;
		for (let i = 0; i < length; i++) chars[i] = digits[i] ?? '';
		els[Math.min(digits.length, length - 1)]?.focus();
	}
</script>

<div class="flex justify-center gap-2" role="group" aria-label={t('otp.group')}>
	{#each chars as ch, i (i)}
		<input
			bind:this={els[i]}
			value={ch}
			inputmode="numeric"
			autocomplete={i === 0 ? 'one-time-code' : 'off'}
			maxlength="1"
			aria-label={t('otp.digit', { n: i + 1 })}
			class="input w-12 px-0 text-center font-mono text-lg focus:outline-none"
			class:input-error={invalid}
			oninput={(e) => onInput(i, e)}
			onkeydown={(e) => onKeydown(i, e)}
			onpaste={onPaste}
		/>
	{/each}
</div>
