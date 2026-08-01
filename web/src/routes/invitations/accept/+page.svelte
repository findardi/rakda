<script lang="ts">
	import { enhance } from '$app/forms';
	import { AuthShell, Field, PasswordField, Button, Alert } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let username = $state('');
	let submitting = $state(false);

	// A 404 on submit (token consumed between preview and accept) flips to invalid.
	let invalid = $derived(data.invalid || form?.invalid === true);
</script>

<svelte:head><title>{t('invAccept.title')} · Riksa</title></svelte:head>

<AuthShell>
	{#if invalid}
		<section class="flex flex-col gap-6 text-center">
			<header>
				<h1 class="text-[1.625rem] font-semibold tracking-[-0.02em] text-balance">
					{t('invAccept.invalid.title')}
				</h1>
				<p class="mt-1.5 text-[0.9375rem] text-muted">{t('invAccept.invalid.body')}</p>
			</header>
			<a href="/login" class="font-medium text-primary hover:underline">{t('invAccept.toLogin')}</a>
		</section>
	{:else if data.loggedIn}
		<section class="flex flex-col gap-6 text-center">
			<header>
				<h1 class="text-[1.625rem] font-semibold tracking-[-0.02em] text-balance">
					{t('invAccept.loggedIn.title')}
				</h1>
				<p class="mt-1.5 text-[0.9375rem] text-muted">{t('invAccept.loggedIn.body')}</p>
			</header>
			<a href="/invitation" class="font-medium text-primary hover:underline"
				>{t('invAccept.toInbox')}</a
			>
		</section>
	{:else if data.preview}
		<section class="flex flex-col gap-6 text-center">
			<header>
				<h1 class="text-[1.625rem] font-semibold tracking-[-0.02em] text-balance">
					{t('invAccept.title')}
				</h1>
				<p class="mt-1.5 text-[0.9375rem] text-muted">
					{t('invAccept.subtitle', { name: data.preview.workspace_name })}
				</p>
				<p class="mt-1 text-[0.9375rem] text-muted">
					{t('invAccept.roleLine', { role: data.preview.role_name })}
				</p>
			</header>

			{#if form?.message}
				<Alert variant="error">{form.message}</Alert>
			{/if}

			<!-- Locked email — fixed to the invitation, not editable. -->
			<div class="flex flex-col gap-1.5 text-left">
				<span class="text-sm font-medium">{t('invAccept.emailLabel')}</span>
				<div
					class="rounded-(--radius-field) border border-base-content/10 bg-base-200/50 px-3 py-2.5"
				>
					<span class="truncate font-mono text-sm">{data.preview.email}</span>
				</div>
				<p class="text-sm text-muted">{t('invAccept.emailLocked')}</p>
			</div>

			<form
				method="POST"
				action="?/accept"
				novalidate
				class="flex flex-col gap-[1.1rem] text-left"
				use:enhance={() => {
					submitting = true;
					return async ({ update }) => {
						await update({ reset: false });
						submitting = false;
					};
				}}
			>
				<input type="hidden" name="token" value={data.token} />
				<input type="hidden" name="workspace_name" value={data.preview.workspace_name} />
				<Field
					id="username"
					name="username"
					label={t('register.username')}
					autocomplete="username"
					hint={t('register.usernameHint')}
					autofocus
					bind:value={username}
					error={form?.fieldErrors?.username}
				/>
				<PasswordField
					id="password"
					name="password"
					label={t('register.password')}
					autocomplete="new-password"
					hint={t('register.passwordHint')}
					error={form?.fieldErrors?.password}
				/>
				<Button type="submit" full loading={submitting}>
					{submitting ? t('invAccept.submitting') : t('invAccept.submit')}
				</Button>
			</form>

			<div class="flex flex-col items-center gap-2 text-center">
				<p class="text-[0.9375rem] text-muted">
					{t('nav.toLogin')}
					<a href="/login" class="font-medium text-primary hover:underline">{t('nav.toLoginCta')}</a
					>
				</p>
			</div>
		</section>
	{/if}
</AuthShell>
