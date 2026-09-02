<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Field, TextareaField, Toaster, showToast } from '$lib/components/common';
	import { canManageGroups } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { GroupWorkspaceData, MyAccessWorkspace } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const groups = $derived(data.groups);
	const canManage = $derived(
		canManageGroups((page.data as { access?: MyAccessWorkspace }).access?.role ?? '')
	);

	const base = $derived(`/workspace/${page.params.slug}/management-access/group`);

	// --- Create / Edit (one dialog, mode driven by `editing`) ---
	let formDialog = $state<HTMLDialogElement>();
	let editing = $state<GroupWorkspaceData | null>(null);
	let name = $state('');
	let description = $state('');
	let formSubmitting = $state(false);
	let formMessage = $state<string | null>(null);

	function openCreate() {
		editing = null;
		name = '';
		description = '';
		formMessage = null;
		formDialog?.showModal();
	}

	function openEdit(g: GroupWorkspaceData) {
		editing = g;
		name = g.name;
		description = g.description;
		formMessage = null;
		formDialog?.showModal();
	}

	const submitForm: SubmitFunction = () => {
		formSubmitting = true;
		return async ({ result }) => {
			formSubmitting = false;
			if (result.type === 'success') {
				const wasEditing = !!editing;
				formDialog?.close();
				await invalidateAll();
				showToast(wasEditing ? t('group.updated') : t('group.created'), 'success');
			} else if (result.type === 'failure') {
				formMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				formMessage = t('err.generic');
			}
		};
	};

	// --- Delete ---
	let deleteDialog = $state<HTMLDialogElement>();
	let pending = $state<GroupWorkspaceData | null>(null);
	let deleteSubmitting = $state(false);
	let deleteMessage = $state<string | null>(null);

	function openDelete(g: GroupWorkspaceData) {
		pending = g;
		deleteMessage = null;
		deleteDialog?.showModal();
	}

	const submitDelete: SubmitFunction = () => {
		deleteSubmitting = true;
		return async ({ result }) => {
			deleteSubmitting = false;
			if (result.type === 'success') {
				deleteDialog?.close();
				await invalidateAll();
				showToast(t('group.deleted'), 'success');
			} else if (result.type === 'failure') {
				deleteMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				deleteMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head><title>{t('ma.group')} · {t('ma.title')}</title></svelte:head>

<div class="flex items-center justify-between gap-4">
	<h2 class="text-sm font-semibold">
		{t('ma.group')}
		<span class="ml-1 font-mono text-xs font-normal text-muted">{groups.length}</span>
	</h2>
	{#if canManage}
		<button type="button" onclick={openCreate} class="btn btn-primary btn-sm">
			<svg
				class="h-4 w-4"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<path d="M12 5v14M5 12h14" />
			</svg>
			{t('group.new')}
		</button>
	{/if}
</div>

{#if groups.length}
	<ul class="mt-4 divide-y divide-base-content/10 border-y border-base-content/10">
		{#each groups as group (group.id)}
			<li class="flex items-center gap-4 py-3">
				<div class="min-w-0 flex-1">
					<a
						href="{base}/{group.id}"
						class="inline-flex max-w-full items-center gap-1 text-[0.9375rem] font-medium transition-colors hover:text-primary"
					>
						<span class="truncate">{group.name}</span>
						{#if group.is_default}
							<span
								class="ml-1 flex-none rounded-selector bg-base-content/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted"
							>
								{t('group.default.badge')}
							</span>
						{/if}
						<svg
							class="h-3.5 w-3.5 flex-none text-muted"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.8"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="m9 18 6-6-6-6" />
						</svg>
					</a>
					{#if group.is_default}
						<p class="mt-0.5 max-w-[60ch] text-xs text-muted text-pretty">
							{t('group.default.note')}
						</p>
					{:else if group.description}
						<p class="mt-0.5 max-w-[60ch] truncate text-xs text-muted">{group.description}</p>
					{/if}
				</div>

				{#if canManage}
					<div class="flex flex-none items-center gap-1">
						<button
							type="button"
							onclick={() => openEdit(group)}
							class="inline-flex items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm font-medium text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
						>
							<svg
								class="h-4 w-4"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="1.8"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
							>
								<path d="M12 20h9" />
								<path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
							</svg>
							{t('group.edit')}
						</button>
						{#if !group.is_default}
							<button
								type="button"
								onclick={() => openDelete(group)}
								class="inline-flex items-center gap-1.5 rounded-field px-2.5 py-2.5 text-sm font-medium text-muted transition-colors hover:bg-error/10 hover:text-error"
							>
								<svg
									class="h-4 w-4"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="1.8"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"
								>
									<path
										d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"
									/>
									<path d="M10 11v6M14 11v6" />
								</svg>
								{t('group.delete')}
							</button>
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ul>
{:else}
	<div class="mt-4 rounded-box border border-dashed border-base-content/15 px-6 py-10 text-center">
		<p class="text-sm font-medium">{t('group.empty.title')}</p>
		<p class="mx-auto mt-1 max-w-[48ch] text-sm text-muted text-pretty">{t('group.empty.body')}</p>
		{#if canManage}
			<button type="button" onclick={openCreate} class="btn btn-primary btn-sm mt-4">
				<svg
					class="h-4 w-4"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M12 5v14M5 12h14" />
				</svg>
				{t('group.new')}
			</button>
		{/if}
	</div>
{/if}

<!-- Create / Edit -->
<dialog bind:this={formDialog} class="modal" aria-labelledby="group-form-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="group-form-title" class="text-lg font-semibold tracking-[-0.01em]">
			{editing ? t('group.edit.title') : t('group.create.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('group.create.desc')}</p>

		{#if formMessage}
			<div class="mt-4"><Alert align="start">{formMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action={editing ? '?/update' : '?/create'}
			use:enhance={submitForm}
			class="mt-5 flex flex-col gap-4"
		>
			{#if editing}
				<input type="hidden" name="groupId" value={editing.id} />
			{/if}
			<Field
				id="group-name"
				name="name"
				label={t('group.field.name')}
				placeholder={t('group.field.namePlaceholder')}
				bind:value={name}
				required
				maxlength={80}
				autofocus
			/>
			<TextareaField
				id="group-description"
				name="description"
				label={t('group.field.description')}
				placeholder={t('group.field.descriptionPlaceholder')}
				bind:value={description}
				maxlength={280}
				rows={3}
			/>

			<div class="mt-1 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => formDialog?.close()}>
					{t('group.cancel')}
				</Button>
				<Button type="submit" loading={formSubmitting} disabled={!name.trim()}>
					{#if editing}
						{formSubmitting ? t('group.saving') : t('group.save')}
					{:else}
						{formSubmitting ? t('group.create.submitting') : t('group.create.submit')}
					{/if}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('group.cancel')}></button>
	</form>
</dialog>

<!-- Delete confirm -->
<dialog bind:this={deleteDialog} class="modal" aria-labelledby="group-delete-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="group-delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('group.delete.title')}
		</h2>
		{#if pending}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('group.delete.warning', { name: pending.name })}
			</p>
		{/if}

		{#if deleteMessage}
			<div class="mt-4"><Alert align="start">{deleteMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/delete"
			use:enhance={submitDelete}
			class="mt-5 flex flex-wrap justify-end gap-2"
		>
			<input type="hidden" name="groupId" value={pending?.id ?? ''} />
			<Button type="button" variant="ghost" onclick={() => deleteDialog?.close()}>
				{t('group.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={deleteSubmitting}>
				{deleteSubmitting ? t('group.delete.submitting') : t('group.delete.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('group.cancel')}></button>
	</form>
</dialog>

<Toaster />
