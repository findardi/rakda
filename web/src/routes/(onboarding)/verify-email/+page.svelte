<script lang="ts">
	import { enhance } from '$app/forms';
	import { onDestroy } from 'svelte';
	import { OtpInput, Alert, Button } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let otp = $state('');
	let verifying = $state(false);
	let resending = $state(false);
	let justResent = $state(false);

	// Politeness cooldown on resend (server-side rate-limit is a separate concern).
	let cooldown = $state(0);
	let timer: ReturnType<typeof setInterval> | undefined;
	function startCooldown() {
		cooldown = 60;
		clearInterval(timer);
		timer = setInterval(() => {
			cooldown -= 1;
			if (cooldown <= 0) {
				cooldown = 0;
				clearInterval(timer);
			}
		}, 1000);
	}
	onDestroy(() => clearInterval(timer));
</script>

<svelte:head><title>{t('verify.title')} · Rakda</title></svelte:head>

<section class="flex flex-col gap-6 text-center">
	<header>
		<h1 class="text-[1.625rem] font-semibold tracking-[-0.02em] text-balance">
			{t('verify.title')}
		</h1>
		<p class="mt-1.5 text-[0.9375rem] text-muted">{t('verify.subtitle', { email: data.email })}</p>
	</header>

	{#if form?.message}
		<Alert variant="error">{form.message}</Alert>
	{:else}
		<Alert variant="success">{t('verify.sent')}</Alert>
	{/if}

	<div class="flex flex-col gap-3">
		<span class="text-sm font-medium">{t('verify.otpTitle')}</span>
		<OtpInput bind:value={otp} invalid={!!form?.fieldErrors?.code} autofocus />
		{#if form?.fieldErrors?.code}
			<p class="text-sm text-error">{form.fieldErrors.code}</p>
		{/if}

		<form
			method="POST"
			action="?/verify"
			use:enhance={() => {
				verifying = true;
				return async ({ update }) => {
					await update({ reset: false });
					verifying = false;
				};
			}}
		>
			<input type="hidden" name="code" value={otp} />
			<Button type="submit" full loading={verifying} disabled={otp.length < 6}>
				{verifying ? t('verify.verifying') : t('verify.submit')}
			</Button>
		</form>

		<div class="flex items-center justify-center gap-2 text-sm">
			<span class="text-muted">{t('verify.noCode')}</span>
			<form
				method="POST"
				action="?/resend"
				use:enhance={() => {
					resending = true;
					justResent = false;
					return async ({ result, update }) => {
						if (result.type === 'success') {
							justResent = true;
							startCooldown();
						}
						await update({ reset: false });
						resending = false;
					};
				}}
			>
				<button
					type="submit"
					class="font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:text-muted disabled:no-underline"
					disabled={cooldown > 0 || resending}
				>
					{resending
						? t('verify.resending')
						: cooldown > 0
							? t('verify.resendIn', { s: cooldown })
							: t('verify.resend')}
				</button>
			</form>
		</div>
		{#if justResent}<p class="text-sm text-success">{t('verify.resent')}</p>{/if}
	</div>

	<form method="POST" action="?/logout" use:enhance>
		<button type="submit" class="text-[0.9375rem] font-medium text-primary hover:underline">
			{t('verify.logout')}
		</button>
	</form>
</section>
