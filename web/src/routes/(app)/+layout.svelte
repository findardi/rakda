<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { AppSidebar, AppTopbar, RoomSidebar } from '$lib/components/app';
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
			{#if room}
				<RoomSidebar workspace={room} {access} {qaWaiting} />
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
			{#if room}
				<RoomSidebar workspace={room} {access} {qaWaiting} />
			{:else}
				<AppSidebar invitations={data.invitationCount} />
			{/if}
		</aside>

		<main class="min-w-0 flex-1 overflow-y-auto">
			{@render children()}
		</main>
	</div>
</div>
