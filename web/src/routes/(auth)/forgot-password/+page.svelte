<script lang="ts">
	import { enhance } from '$app/forms';
	import { onDestroy } from 'svelte';
	import { Field, OtpInput, PasswordField, Button, Alert } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();

	let email = $state('');
	let otp = $state('');
	let sent = $state(false);
	let verified = $state(false);
	let sending = $state(false);
	let resending = $state(false);
	let verifying = $state(false);
	let resetting = $state(false);
	let justResent = $state(false);

	let newPassword = $state('');
	let confirmPassword = $state('');
	// Live mismatch check — surfaced before the round-trip to the server.
	const passwordsMismatch = $derived(confirmPassword.length > 0 && newPassword !== confirmPassword);

	let secondsLeft = $state(0);
	let timer: ReturnType<typeof setInterval> | undefined;

	function startCountdown() {
		secondsLeft = 300;
		clearInterval(timer);
		timer = setInterval(() => {
			secondsLeft -= 1;
			if (secondsLeft <= 0) {
				secondsLeft = 0;
				clearInterval(timer);
			}
		}, 1000);
	}
	onDestroy(() => clearInterval(timer));

	const countdown = $derived(
		`${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, '0')}`
	);
</script>

<svelte:head><title>{t('forgot.title')} · Rakda</title></svelte:head>

<section class="flex flex-col gap-6 text-center">
	<header>
		<h1 class="text-[1.625rem] font-semibold tracking-[-0.02em] text-balance">
			{t('forgot.title')}
		</h1>
		<p class="mt-1.5 text-[0.9375rem] text-muted">
			{verified
				? t('reset.subtitle', { email })
				: sent
					? t('forgot.otpSubtitle', { email })
					: t('forgot.subtitle')}
		</p>
	</header>

	{#if !sent}
		<!-- Step 1 — email -->
		{#if form?.message}
			<Alert variant="error">{form.message}</Alert>
		{/if}
		<form
			method="POST"
			action="?/send"
			novalidate
			class="flex flex-col gap-[1.1rem] text-left"
			use:enhance={() => {
				sending = true;
				return async ({ result, update }) => {
					if (result.type === 'success' && (result.data as { sent?: boolean })?.sent) {
						sent = true;
						startCountdown();
					}
					await update({ reset: false });
					sending = false;
				};
			}}
		>
			<Field
				id="email"
				name="email"
				type="email"
				label={t('forgot.email')}
				autocomplete="email"
				inputmode="email"
				autofocus
				bind:value={email}
				error={form?.fieldErrors?.email}
			/>
			<Button type="submit" full loading={sending}>
				{sending ? t('forgot.sending') : t('forgot.send')}
			</Button>
		</form>
		<div class="flex flex-col items-center gap-2 text-center">
			<p class="text-[0.9375rem] text-muted">
				{t('nav.toLogin')}
				<a href="/login" class="font-medium text-primary hover:underline">{t('nav.toLoginCta')}</a>
			</p>
		</div>
	{:else if !verified}
		<!-- Step 2 — OTP -->
		{#if form?.message}
			<Alert variant="error">{form.message}</Alert>
		{/if}

		<div class="flex flex-col gap-3">
			<form
				method="POST"
				action="?/verify"
				class="flex flex-col items-center gap-3"
				use:enhance={() => {
					verifying = true;
					return async ({ result, update }) => {
						if (result.type === 'success' && (result.data as { valid?: boolean })?.valid) {
							verified = true;
							clearInterval(timer);
						}
						await update({ reset: false });
						verifying = false;
					};
				}}
			>
				<input type="hidden" name="email" value={email} />
				<label for="otp" class="text-sm font-medium">{t('forgot.otpTitle')}</label>
				<OtpInput bind:value={otp} invalid={!!form?.fieldErrors?.code} autofocus />
				{#if form?.fieldErrors?.code}
					<p id="otp-error" role="alert" class="text-sm text-error">{form.fieldErrors.code}</p>
				{/if}
				<div class="mt-1 w-full">
					<Button type="submit" full loading={verifying} disabled={otp.length < 6}>
						{verifying ? t('forgot.verifying') : t('forgot.verify')}
					</Button>
				</div>
			</form>

			<div class="flex items-center justify-center gap-5 text-sm">
				<span class="text-muted">
					{#if secondsLeft > 0}
						{t('forgot.expiresIn', { time: countdown })}
					{:else}
						{t('forgot.expired')}
					{/if}
				</span>
				<form
					method="POST"
					action="?/send"
					use:enhance={() => {
						resending = true;
						justResent = false;
						return async ({ result, update }) => {
							if (result.type === 'success' && (result.data as { sent?: boolean })?.sent) {
								startCountdown();
								justResent = true;
							}
							await update({ reset: false });
							resending = false;
						};
					}}
				>
					<input type="hidden" name="email" value={email} />
					<button
						type="submit"
						class="font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:text-muted disabled:no-underline"
						disabled={secondsLeft > 0 || resending}
					>
						{resending ? t('forgot.sending') : t('forgot.resend')}
					</button>
				</form>
			</div>
			{#if justResent}
				<p class="text-sm text-success" role="status">{t('forgot.resent')}</p>
			{/if}
		</div>

		<div class="flex items-center justify-center text-center">
			<button
				type="button"
				class="text-[0.9375rem] font-medium text-primary hover:underline"
				onclick={() => {
					sent = false;
					clearInterval(timer);
				}}
			>
				{t('forgot.changeEmail')}
			</button>
			<div class="divider divider-horizontal"></div>
			<a href="/login" class="text-[0.9375rem] font-medium text-primary hover:underline">
				{t('forgot.back')}
			</a>
		</div>
	{:else}
		<!-- Step 3 — reset password -->
		<form
			method="POST"
			action="?/reset"
			novalidate
			class="flex flex-col gap-[1.1rem] text-left"
			use:enhance={() => {
				resetting = true;
				return async ({ result, update }) => {
					if (
						result.type === 'failure' &&
						(result.data as { invalidCode?: boolean })?.invalidCode
					) {
						verified = false;
						secondsLeft = 0;
						clearInterval(timer);
					}
					await update({ reset: false });
					resetting = false;
				};
			}}
		>
			<input type="hidden" name="email" value={email} />
			<input type="hidden" name="code" value={otp} />
			<PasswordField
				id="password"
				name="password"
				label={t('reset.newPassword')}
				autocomplete="new-password"
				hint={t('reset.passwordHint')}
				autofocus
				bind:value={newPassword}
				error={form?.fieldErrors?.password}
			/>
			<PasswordField
				id="confirm"
				name="confirm"
				label={t('reset.confirmPassword')}
				autocomplete="new-password"
				bind:value={confirmPassword}
				error={passwordsMismatch ? t('reset.mismatch') : form?.fieldErrors?.confirm}
			/>
			<Button type="submit" full loading={resetting} disabled={passwordsMismatch}>
				{resetting ? t('reset.submitting') : t('reset.submit')}
			</Button>
		</form>

		<p class="text-[0.9375rem] text-muted">
			<a href="/login" class="font-medium text-primary hover:underline">{t('forgot.back')}</a>
		</p>
	{/if}
</section>
