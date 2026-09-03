<script lang="ts">
	import { tick } from 'svelte';
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { WorkspaceStatusBadge } from '$lib/components/app';
	import { Alert, Button, Field, TextareaField } from '$lib/components/common';
	import { workspaceLogoUrl } from '$lib/branding';
	import { t } from '$lib/i18n';
	import type { WorkspaceData } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let dialog = $state<HTMLDialogElement>();
	let alertEl = $state<HTMLDivElement>();
	let submitting = $state(false);
	let name = $state('');
	let description = $state('');
	let formMessage = $state<string | null>(null);
	let fieldErrors = $state<Record<string, string>>({});

	// Success feedback — transient toast + brief highlight of the new row.
	let toast = $state<string | null>(null);
	let highlightId = $state<string | null>(null);
	let toastTimer: ReturnType<typeof setTimeout>;

	// The owned-room limit applies only to rooms the user OWNS; as a guest they
	// can be in more. Both numbers are server-computed — never derived here.
	const atLimit = $derived(data.ownedLimit > 0 && data.ownedCount >= data.ownedLimit);

	async function openCreate() {
		name = '';
		description = '';
		formMessage = null;
		fieldErrors = {};
		dialog?.showModal();
		await tick();
		dialog?.querySelector<HTMLInputElement>('#ws-name')?.focus();
	}

	function focusFirstError() {
		if (fieldErrors.name) dialog?.querySelector<HTMLInputElement>('#ws-name')?.focus();
		else if (formMessage) alertEl?.focus();
	}

	function showSuccess(created: WorkspaceData) {
		toast = t('ws.created', { name: created.name });
		highlightId = created.id;
		clearTimeout(toastTimer);
		toastTimer = setTimeout(() => {
			toast = null;
			highlightId = null;
		}, 4500);
	}

	const submitCreate: SubmitFunction = () => {
		submitting = true;
		return async ({ result }) => {
			submitting = false;
			if (result.type === 'success') {
				const created = result.data?.created as WorkspaceData | undefined;
				dialog?.close();
				await invalidateAll();
				if (created) showSuccess(created);
			} else if (result.type === 'failure') {
				formMessage = (result.data?.message as string | null) ?? null;
				fieldErrors = (result.data?.fieldErrors as Record<string, string>) ?? {};
				await tick();
				focusFirstError();
			} else if (result.type === 'redirect') {
				await applyAction(result); // e.g. session expired → /login
			} else {
				formMessage = t('err.generic');
				await tick();
				focusFirstError();
			}
		};
	};

	const dateFmt = new Intl.DateTimeFormat('id-ID', {
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	});
	const fmtDate = (iso: string) => dateFmt.format(new Date(iso));

	const ROLE_KEY = {
		owner: 'role.sys.owner',
		admin: 'role.sys.admin',
		guest: 'role.sys.guest'
	} as const;
	const roleLabel = (role?: string) =>
		role && role in ROLE_KEY ? t(ROLE_KEY[role as keyof typeof ROLE_KEY]) : '';
</script>

<svelte:head><title>{t('ws.title')} · {t('brand.name')}</title></svelte:head>

<div class="mx-auto w-full max-w-3xl px-6 py-8">
	<header class="flex items-start justify-between gap-4">
		<div>
			<h1 class="text-xl font-semibold tracking-[-0.015em]">{t('ws.title')}</h1>
			{#if data.workspaces.length}
				<p class="mt-0.5 text-sm text-muted">{t('ws.count', { n: data.workspaces.length })}</p>
			{/if}
		</div>
		<Button onclick={openCreate} disabled={atLimit}>{t('ws.create')}</Button>
	</header>

	{#if atLimit}
		<p class="mt-3 text-sm text-muted">{t('ws.limitReached', { limit: data.ownedLimit })}</p>
	{/if}

	{#if data.loadError}
		<div class="mt-6"><Alert align="start">{t('ws.loadError')}</Alert></div>
	{:else if data.workspaces.length === 0}
		<div class="mt-10 grid place-items-center px-6 py-16">
			<section class="flex max-w-md flex-col items-center text-center">
				<span
					class="mb-5 grid h-14 w-14 place-items-center rounded-box border border-base-content/10 bg-base-100 text-muted"
				>
					<svg
						class="h-6 w-6"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<rect x="3" y="3" width="7" height="7" rx="1.5" />
						<rect x="14" y="3" width="7" height="7" rx="1.5" />
						<rect x="3" y="14" width="7" height="7" rx="1.5" />
						<rect x="14" y="14" width="7" height="7" rx="1.5" />
					</svg>
				</span>
				<h2 class="text-[1.375rem] font-semibold tracking-[-0.015em] text-balance">
					{t('ws.empty.title')}
				</h2>
				<p class="mt-2 max-w-[42ch] text-[0.9375rem] leading-relaxed text-muted text-pretty">
					{t('ws.empty.body')}
				</p>
				<div class="mt-6">
					<Button onclick={openCreate}>{t('ws.create')}</Button>
				</div>
			</section>
		</div>
	{:else}
		<!-- A room row preloads the whole room subtree (layout + overview, 6–7
		     upstream reads), so wait for mousedown instead of every pass of the pointer. -->
		<ul
			data-sveltekit-preload-data="tap"
			class="mt-6 divide-y divide-base-content/10 overflow-hidden rounded-box border border-base-content/10 bg-base-100"
		>
			{#each data.workspaces as ws (ws.id)}
				<li>
					<a
						href="/workspace/{ws.slug}"
						class="flex items-center gap-4 px-4 py-3.5 transition-colors duration-200 hover:bg-base-content/5 {ws.id ===
						highlightId
							? 'bg-primary/5'
							: ''}"
					>
						{#if ws.logo}
							<img
								src={workspaceLogoUrl(ws)}
								alt=""
								class="h-9 w-9 flex-none rounded-field border border-base-content/10 bg-base-100 object-contain p-0.5"
							/>
						{:else}
							<span
								class="grid h-9 w-9 flex-none place-items-center rounded-field bg-primary/10 text-sm font-semibold text-primary"
								>{ws.name.charAt(0).toUpperCase()}</span
							>
						{/if}
						<div class="min-w-0 flex-1">
							<p class="truncate text-[0.9375rem] font-medium">{ws.name}</p>
							<p class="truncate font-mono text-xs text-muted">{ws.slug}</p>
						</div>
						{#if ws.description}
							<p class="hidden max-w-[36ch] flex-1 truncate text-sm text-muted md:block">
								{ws.description}
							</p>
						{/if}
						{#if roleLabel(ws.role)}
							<span class="hidden flex-none text-xs text-muted sm:inline">{roleLabel(ws.role)}</span
							>
						{/if}
						<WorkspaceStatusBadge status={ws.status} class="flex-none" />
						<span
							class="hidden flex-none font-mono text-xs text-muted sm:inline"
							title={ws.last_activity_at ? t('ws.lastActivity') : t('ws.detail.created')}
						>
							{fmtDate(ws.last_activity_at ?? ws.created_at)}
						</span>
						<svg
							class="h-4 w-4 flex-none text-muted"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.6"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="m9 6 6 6-6 6" />
						</svg>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<dialog
	bind:this={dialog}
	class="modal"
	aria-labelledby="ws-create-title"
	aria-describedby="ws-create-subtitle"
>
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="ws-create-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('ws.dialog.title')}
		</h2>
		<p id="ws-create-subtitle" class="mt-1 text-sm text-muted">{t('ws.dialog.subtitle')}</p>

		<p class="mt-3 flex items-start gap-2 rounded-field bg-base-200 px-3 py-2 text-xs text-muted">
			<svg
				class="mt-px h-4 w-4 flex-none text-primary"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<rect x="5" y="11" width="14" height="9" rx="2" />
				<path d="M8 11V8a4 4 0 0 1 8 0v3" />
			</svg>
			<span>{t('ws.dialog.reassure')}</span>
		</p>

		{#if formMessage}
			<div bind:this={alertEl} tabindex="-1" class="mt-4 outline-none">
				<Alert align="start">{formMessage}</Alert>
			</div>
		{/if}

		<form
			method="POST"
			action="?/create"
			use:enhance={submitCreate}
			class="mt-4 flex flex-col gap-4"
		>
			<Field
				id="ws-name"
				name="name"
				label={t('ws.field.name')}
				bind:value={name}
				placeholder={t('ws.field.namePlaceholder')}
				required
				maxlength={120}
				error={fieldErrors.name}
			/>

			<TextareaField
				id="ws-description"
				name="description"
				label={t('ws.field.description')}
				bind:value={description}
				placeholder={t('ws.field.descriptionPlaceholder')}
				hint={t('ws.field.descriptionHint')}
				maxlength={500}
				error={fieldErrors.description}
			/>

			<div class="mt-2 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => dialog?.close()}>
					{t('ws.dialog.cancel')}
				</Button>
				<Button type="submit" loading={submitting}>
					{submitting ? t('ws.dialog.submitting') : t('ws.dialog.submit')}
				</Button>
			</div>
		</form>
	</div>

	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

{#if toast}
	<div class="pointer-events-none fixed inset-x-0 bottom-4 z-toast flex justify-center px-4">
		<div class="pointer-events-auto"><Alert variant="success">{toast}</Alert></div>
	</div>
{/if}
