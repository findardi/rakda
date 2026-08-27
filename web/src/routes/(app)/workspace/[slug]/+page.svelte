<script lang="ts">
	import { tick } from 'svelte';
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, Field, TextareaField, Toaster, showToast } from '$lib/components/common';
	import { WorkspaceStatusBadge } from '$lib/components/app';
	import {
		canDeleteWorkspace,
		canEditWorkspace,
		canTransitionRoom,
		isRoomOpenTo,
		isRoomReadOnly
	} from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { MyAccessWorkspace, WorkspaceStatus } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const ws = $derived(data.workspace);

	// Edit, status, and delete are all RequireOwner on the backend — owner only.
	const role = $derived((data as { access?: MyAccessWorkspace }).access?.role ?? '');
	const canEdit = $derived(canEditWorkspace(role));
	const canDelete = $derived(canDeleteWorkspace(role));

	// One source for the gate: `roomStatus` comes from the same membership query
	// the server guards read, so the UI can never offer what the API refuses.
	const status = $derived((data as { roomStatus?: WorkspaceStatus }).roomStatus ?? ws.status);
	const roomOpen = $derived(isRoomOpenTo(status, role));
	const readOnly = $derived(isRoomReadOnly(status));
	const guestCount = $derived((data as { guestCount?: number }).guestCount ?? 0);

	const dateFmt = new Intl.DateTimeFormat('id-ID', {
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	});
	const fmtDate = (iso: string) => dateFmt.format(new Date(iso));

	const statusHint = $derived(t(`ws.status.hint.${status}`));

	// --- Status change ---
	let pendingStatus = $state<WorkspaceStatus | null>(null);
	let archiveDialog = $state<HTMLDialogElement>();
	let archiveMessage = $state<string | null>(null);

	function openArchive() {
		archiveMessage = null;
		archiveDialog?.showModal();
	}

	const submitStatus: SubmitFunction = ({ formData }) => {
		pendingStatus = formData.get('status') as WorkspaceStatus;
		archiveMessage = null;
		return async ({ result }) => {
			if (result.type === 'success') {
				archiveDialog?.close();
				await invalidateAll();
				showToast(t('ws.status.updated'), 'success');
			} else if (result.type === 'redirect') {
				await applyAction(result);
			} else if (result.type === 'failure') {
				const msg = (result.data?.message as string) ?? t('err.generic');
				if (pendingStatus === 'archive' && archiveDialog?.open) archiveMessage = msg;
				else showToast(msg, 'error');
			} else {
				showToast(t('err.generic'), 'error');
			}
			pendingStatus = null;
		};
	};

	// --- Edit name & description ---
	let editDialog = $state<HTMLDialogElement>();
	let editAlertEl = $state<HTMLDivElement>();
	let editName = $state('');
	let editDescription = $state('');
	let editSubmitting = $state(false);
	let editMessage = $state<string | null>(null);
	let editFieldErrors = $state<Record<string, string>>({});

	function openEdit() {
		editName = ws.name;
		editDescription = ws.description ?? '';
		editMessage = null;
		editFieldErrors = {};
		editDialog?.showModal();
		tick().then(() => editDialog?.querySelector<HTMLInputElement>('#edit-name')?.focus());
	}

	function focusEditError() {
		if (editFieldErrors.name) editDialog?.querySelector<HTMLInputElement>('#edit-name')?.focus();
		else if (editMessage) editAlertEl?.focus();
	}

	const submitEdit: SubmitFunction = () => {
		editSubmitting = true;
		return async ({ result }) => {
			editSubmitting = false;
			if (result.type === 'success') {
				editDialog?.close();
				await invalidateAll();
				showToast(t('ws.edit.saved'), 'success');
			} else if (result.type === 'redirect') {
				editDialog?.close();
				await applyAction(result); // reslugged — lands on the new slug
			} else if (result.type === 'failure') {
				editMessage = (result.data?.message as string | null) ?? null;
				editFieldErrors = (result.data?.fieldErrors as Record<string, string>) ?? {};
				await tick();
				focusEditError();
			} else {
				editMessage = t('err.generic');
				await tick();
				focusEditError();
			}
		};
	};

	// --- Delete (type-to-confirm) ---
	let deleteDialog = $state<HTMLDialogElement>();
	let deleteConfirm = $state('');
	let deleteSubmitting = $state(false);
	let deleteMessage = $state<string | null>(null);
	const deleteReady = $derived(deleteConfirm.trim() === ws.name);

	function openDelete() {
		deleteConfirm = '';
		deleteMessage = null;
		deleteDialog?.showModal();
	}

	const submitDelete: SubmitFunction = ({ cancel }) => {
		if (!deleteReady) return cancel();
		deleteSubmitting = true;
		return async ({ result }) => {
			deleteSubmitting = false;
			if (result.type === 'redirect') {
				deleteDialog?.close();
				await applyAction(result); // → /workspace
			} else if (result.type === 'failure') {
				deleteMessage = (result.data?.message as string) ?? t('err.generic');
			} else {
				deleteMessage = t('err.generic');
			}
		};
	};
</script>

<svelte:head><title>{ws.name} · {t('brand.name')}</title></svelte:head>

{#if !roomOpen}
	<div class="mx-auto w-full max-w-2xl px-6 py-8">
		<h1 class="truncate text-2xl font-semibold tracking-[-0.02em]">{ws.name}</h1>
		<div class="mt-6 rounded-box border border-base-content/10 bg-base-100 p-6">
			<h2 class="text-sm font-semibold">{t('room.notOpen.title')}</h2>
			<p class="mt-1.5 max-w-[52ch] text-sm text-muted text-pretty">{t('room.notOpen.body')}</p>
		</div>
	</div>
{:else}
	<div class="mx-auto w-full max-w-2xl px-6 py-8">
		<header class="flex items-start justify-between gap-4">
			<div class="min-w-0">
				<h1 class="truncate text-2xl font-semibold tracking-[-0.02em]">{ws.name}</h1>
				<p
					class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 font-mono text-xs text-muted"
				>
					<span>{ws.slug}</span>
					<span aria-hidden="true">·</span>
					<span>{t('ws.detail.created')} {fmtDate(ws.created_at)}</span>
					<span aria-hidden="true">·</span>
					<span>{t('ws.detail.updated')} {fmtDate(ws.updated_at)}</span>
				</p>
			</div>
			{#if canEdit}
				<Button variant="ghost" onclick={openEdit}>{t('ws.edit.open')}</Button>
			{/if}
		</header>

		{#if ws.description}
			<p class="mt-5 max-w-[65ch] text-[0.9375rem] leading-relaxed text-muted text-pretty">
				{ws.description}
			</p>
		{/if}

		<!-- Status — the badge states it; the controls offer only legal transitions
	     (server map: prepare→active|archive, active→archive, archive→active). -->
		<section class="mt-10">
			<h2 class="text-sm font-semibold">{t('ws.status.label')}</h2>
			<div class="mt-2"><WorkspaceStatusBadge {status} /></div>
			<p class="mt-2 max-w-[52ch] text-sm text-muted text-pretty">{statusHint}</p>

			{#if canEdit}
				{#if status === 'prepare'}
					<div class="mt-5 rounded-box border border-base-content/10 bg-base-100 p-5">
						<h3 class="text-sm font-semibold">{t('room.activate.title')}</h3>
						<p class="mt-1 max-w-[52ch] text-sm text-muted text-pretty">
							{t('room.activate.body')}
						</p>
						<div class="mt-4 flex flex-wrap items-center gap-2">
							<form method="POST" action="?/updateStatus" use:enhance={submitStatus}>
								<input type="hidden" name="status" value="active" />
								<Button type="submit" loading={pendingStatus === 'active'}>
									{t('room.activate.submit')}
								</Button>
							</form>
							{#if canTransitionRoom(status, 'archive')}
								<Button variant="ghost" onclick={openArchive}>{t('room.archive.open')}</Button>
							{/if}
						</div>
					</div>
				{:else if status === 'active'}
					<div class="mt-4">
						<Button variant="ghost" onclick={openArchive}>{t('room.archive.open')}</Button>
					</div>
				{:else if status === 'archive'}
					<form method="POST" action="?/updateStatus" use:enhance={submitStatus} class="mt-4">
						<input type="hidden" name="status" value="active" />
						<Button type="submit" variant="ghost" loading={pendingStatus === 'active'}>
							{t('room.unarchive.submit')}
						</Button>
					</form>
				{/if}
			{/if}
		</section>

		<!-- Delete — quiet settings-style row; the red button carries the danger. -->
		{#if canDelete}
			<section
				class="mt-10 flex flex-col gap-3 border-t border-base-content/10 pt-6 sm:flex-row sm:items-start sm:justify-between sm:gap-6"
			>
				<div class="min-w-0">
					<h2 class="text-sm font-semibold">{t('ws.delete.title')}</h2>
					<p class="mt-1 max-w-[48ch] text-sm text-muted text-pretty">
						{readOnly ? t('room.delete.blocked') : t('ws.delete.body')}
					</p>
				</div>
				<div class="flex-none">
					<Button variant="danger" disabled={readOnly} onclick={openDelete}>
						{t('ws.delete.open')}
					</Button>
				</div>
			</section>
		{/if}
	</div>
{/if}

<!-- Edit dialog -->
<dialog bind:this={editDialog} class="modal" aria-labelledby="edit-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="edit-title" class="text-lg font-semibold tracking-[-0.01em]">{t('ws.edit.title')}</h2>

		{#if editMessage}
			<div bind:this={editAlertEl} tabindex="-1" class="mt-4 outline-none">
				<Alert align="start">{editMessage}</Alert>
			</div>
		{/if}

		<form method="POST" action="?/update" use:enhance={submitEdit} class="mt-4 flex flex-col gap-4">
			<Field
				id="edit-name"
				name="name"
				label={t('ws.field.name')}
				bind:value={editName}
				placeholder={t('ws.field.namePlaceholder')}
				required
				maxlength={120}
				error={editFieldErrors.name}
			/>
			<TextareaField
				id="edit-description"
				name="description"
				label={t('ws.field.description')}
				bind:value={editDescription}
				placeholder={t('ws.field.descriptionPlaceholder')}
				hint={t('ws.field.descriptionHint')}
				maxlength={500}
				error={editFieldErrors.description}
			/>
			<div class="mt-2 flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => editDialog?.close()}>
					{t('ws.dialog.cancel')}
				</Button>
				<Button type="submit" loading={editSubmitting}>
					{editSubmitting ? t('ws.dialog.submitting') : t('ws.edit.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<!-- Delete dialog -->
<dialog bind:this={deleteDialog} class="modal" aria-labelledby="delete-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('ws.delete.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('ws.delete.warning', { name: ws.name })}</p>

		{#if deleteMessage}
			<div class="mt-4"><Alert align="start">{deleteMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/delete"
			use:enhance={submitDelete}
			class="mt-4 flex flex-col gap-4"
		>
			<Field
				id="delete-confirm"
				name="confirm"
				label={t('ws.delete.confirmLabel', { name: ws.name })}
				bind:value={deleteConfirm}
				placeholder={ws.name}
				autocomplete="off"
			/>
			<div class="mt-2 flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => deleteDialog?.close()}>
					{t('ws.dialog.cancel')}
				</Button>
				<Button type="submit" variant="danger" disabled={!deleteReady} loading={deleteSubmitting}>
					{deleteSubmitting ? t('ws.delete.submitting') : t('ws.delete.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<!-- Archive confirmation — counted button, not type-to-confirm: archiving is
     reversible and must not read like deletion. -->
<dialog bind:this={archiveDialog} class="modal" aria-labelledby="archive-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="archive-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('room.archive.title')}
		</h2>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.archive.body')}</p>
		<p class="mt-2 text-sm text-muted text-pretty">{t('room.archive.caveat')}</p>

		{#if archiveMessage}
			<div class="mt-4"><Alert align="start">{archiveMessage}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/updateStatus"
			use:enhance={submitStatus}
			class="mt-5 flex justify-end gap-2"
		>
			<input type="hidden" name="status" value="archive" />
			<Button type="button" variant="ghost" onclick={() => archiveDialog?.close()}>
				{t('ws.dialog.cancel')}
			</Button>
			<Button type="submit" loading={pendingStatus === 'archive'}>
				{guestCount > 0
					? t('room.archive.submitCount', { n: guestCount })
					: t('room.archive.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>

<Toaster />
