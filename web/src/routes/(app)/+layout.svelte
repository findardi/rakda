<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { AppSidebar, AppTopbar, DownloadJobs, RoomSidebar } from '$lib/components/app';
	import { isRoomReadOnly } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { MyAccessWorkspace, WorkspaceData } from '$lib/types/workspace';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	let navOpen = $state(false);

	// Context-swap: inside a room, `page.data.workspace` is set by the room
	// layout load, so the shell shows room nav instead of the global nav.
	const room = $derived((page.data as { workspace?: WorkspaceData }).workspace);
	// The viewer's standing in the open room — drives role-based nav.
	const access = $derived((page.data as { access?: MyAccessWorkspace }).access);
	const qaWaiting = $derived((page.data as { qaWaiting?: number }).qaWaiting ?? 0);
	const qaEnabled = $derived((page.data as { qaEnabled?: boolean }).qaEnabled ?? true);
	const roomStatus = $derived((page.data as { roomStatus?: string }).roomStatus ?? '');
	// A room the viewer may not enter yet has no room nav to show — fall back to
	// the global nav rather than listing destinations that all redirect back.
	const roomOpen = $derived((page.data as { roomOpen?: boolean }).roomOpen ?? true);
	const readOnly = $derived(!!room && isRoomReadOnly(roomStatus));

	// Close the mobile drawer after any navigation.
	afterNavigate(() => (navOpen = false));
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape') navOpen = false;
	}}
/>

<div class="flex h-dvh flex-col bg-base-200">
	<AppTopbar user={data.user} onMenuToggle={() => (navOpen = !navOpen)} />

	<div class="flex min-h-0 flex-1">
		<!-- Desktop: static sidebar — global nav, or room nav inside a room. -->
		<aside class="hidden w-60 shrink-0 border-r border-base-content/10 bg-base-300 md:block">
			{#if room && roomOpen}
				<RoomSidebar workspace={room} {access} {qaWaiting} {qaEnabled} />
			{:else}
				<AppSidebar invitations={data.invitationCount} />
			{/if}
		</aside>

		<!-- Mobile: off-canvas drawer -->
		<div
			class="fixed inset-x-0 top-14 bottom-0 z-backdrop bg-base-content/40 transition-opacity duration-200 motion-reduce:transition-none md:hidden {navOpen
				? 'opacity-100'
				: 'pointer-events-none opacity-0'}"
			onclick={() => (navOpen = false)}
			aria-hidden="true"
		></div>
		<aside
			class="fixed top-14 bottom-0 left-0 z-drawer w-64 border-r border-base-content/10 bg-base-300 transition-transform duration-200 ease-out motion-reduce:transition-none md:hidden {navOpen
				? 'translate-x-0'
				: '-translate-x-full'}"
			aria-label="Navigasi"
			aria-hidden={!navOpen}
		>
			{#if room && roomOpen}
				<RoomSidebar workspace={room} {access} {qaWaiting} {qaEnabled} />
			{:else}
				<AppSidebar invitations={data.invitationCount} />
			{/if}
		</aside>

		<!-- Read-only strip sits beside <main>, not inside it: <main> is the only
		     scroll container, so a child would scroll away. Neutral by DESIGN.md —
		     no alert color, no left stripe; the hollow dot matches the archive badge. -->
		<div class="flex min-w-0 flex-1 flex-col">
			{#if readOnly}
				<div
					role="status"
					class="flex flex-none items-center gap-2 border-b border-base-content/10 bg-base-100 px-6 py-2 text-xs text-muted"
				>
					<span
						class="h-1.5 w-1.5 flex-none rounded-full border border-base-content/40"
						aria-hidden="true"
					></span>
					<span>{t('room.readOnly.strip')}</span>
				</div>
			{/if}

			<main class="min-w-0 flex-1 overflow-y-auto">
				{@render children()}
				<DownloadJobs />
			</main>
		</div>
	</div>
</div>
