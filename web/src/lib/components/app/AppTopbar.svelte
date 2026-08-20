<script lang="ts">
	import { page } from '$app/state';
	import { Brand, LanguageSwitcher } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { MeData } from '$lib/types';
	import CommandPalette from './CommandPalette.svelte';

	type Props = { user: MeData | null; onMenuToggle: () => void };
	let { user, onMenuToggle }: Props = $props();

	// Inside a room the palette searches that room; elsewhere there is no
	// permission context, so the trigger stays disabled.
	const room = $derived((page.data as { workspace?: { id: string; slug: string } }).workspace);
	let paletteOpen = $state(false);

	const initial = $derived((user?.username ?? '?').charAt(0).toUpperCase());

	$effect(() => {
		const handler = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
				if (!room) return;
				const t = e.target as HTMLElement | null;
				if (t?.matches('input, textarea, [contenteditable="true"]')) return;
				e.preventDefault();
				paletteOpen = !paletteOpen;
			}
		};
		window.addEventListener('keydown', handler);
		return () => window.removeEventListener('keydown', handler);
	});
</script>

<header
	class="flex h-14 shrink-0 items-center gap-2 border-b border-base-content/10 bg-base-100 px-3 sm:px-4"
>
	<button
		type="button"
		onclick={onMenuToggle}
		class="rounded-field p-2 text-base-content hover:bg-base-content/5 md:hidden"
		aria-label={t('app.menu.open')}
	>
		<svg
			class="h-5 w-5"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			aria-hidden="true"
		>
			<path d="M4 7h16M4 12h16M4 17h16" />
		</svg>
	</button>

	<a href="/" class="flex items-center rounded-field px-1 py-1" aria-label={t('brand.name')}>
		<Brand size={26} />
	</a>

	<!-- Command palette: opens a room-scoped search (⌘K / Ctrl+K). -->
	<button
		type="button"
		onclick={() => (paletteOpen = true)}
		disabled={!room}
		title={room ? t('app.search.open') : t('app.nav.soon')}
		aria-label={room ? t('app.search.open') : t('app.nav.soon')}
		class="ml-3 hidden max-w-xs flex-1 items-center gap-2 rounded-field border border-base-content/10 bg-base-200 px-3 py-1.5 text-sm text-muted enabled:cursor-pointer enabled:hover:border-base-content/20 disabled:cursor-not-allowed md:flex"
	>
		<svg
			class="h-4 w-4 flex-none"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<circle cx="11" cy="11" r="7" />
			<path d="m21 21-4.3-4.3" />
		</svg>
		<span class="flex-1 text-left">{t('app.search.placeholder')}</span>
		<kbd
			class="rounded border border-base-content/15 bg-base-100 px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted"
			>⌘K</kbd
		>
	</button>

	{#if room}
		<CommandPalette
			open={paletteOpen}
			workspaceId={room.id}
			slug={room.slug}
			onclose={() => (paletteOpen = false)}
		/>
	{/if}

	<div class="dropdown dropdown-end ml-auto">
		<button
			tabindex="0"
			class="flex items-center gap-2 rounded-field py-1 pr-2 pl-1 hover:bg-base-content/5"
		>
			<span
				class="grid h-8 w-8 place-items-center rounded-full bg-primary/12 text-sm font-semibold text-primary"
				>{initial}</span
			>
			<span class="hidden max-w-[14ch] truncate text-sm font-medium sm:block">{user?.username}</span
			>
			<svg
				class="hidden h-4 w-4 text-muted sm:block"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<path d="m6 9 6 6 6-6" />
			</svg>
		</button>
		<ul
			class="dropdown-content z-dropdown mt-2 w-64 rounded-box border border-base-content/10 bg-base-100 p-2 shadow-lg"
		>
			<li class="px-3 py-2">
				<p class="text-xs text-muted">{t('app.account.signedInAs')}</p>
				<p class="truncate text-sm font-medium">{user?.username}</p>
				<p class="truncate font-mono text-xs text-muted">{user?.email}</p>
			</li>
			<li class="mt-1 border-t border-base-content/10 pt-1">
				<LanguageSwitcher variant="inline" />
			</li>
			<li class="mt-1 border-t border-base-content/10 pt-1">
				<form method="POST" action="/?/logout">
					<button
						type="submit"
						class="flex w-full items-center gap-2 rounded-field px-3 py-2 text-left text-sm text-base-content hover:bg-base-content/5"
					>
						<svg
							class="h-4.5 w-4.5 flex-none"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.6"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3" />
							<path d="M10 17l-5-5 5-5" />
							<path d="M5 12h11" />
						</svg>
						{t('app.account.logout')}
					</button>
				</form>
			</li>
		</ul>
	</div>
</header>
