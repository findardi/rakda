<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { workspaceLogoUrl } from '$lib/branding';
	import { formatDateLocal } from '$lib/format';
	import { canManageAccess } from '$lib/access/roles';
	import WorkspaceStatusBadge from './WorkspaceStatusBadge.svelte';
	import type { MyAccessWorkspace, WorkspaceData } from '$lib/types/workspace';

	type Props = {
		workspace: WorkspaceData;
		access?: MyAccessWorkspace;
		qaWaiting?: number;
		qaEnabled?: boolean;
	};
	let { workspace, access, qaWaiting = 0, qaEnabled = true }: Props = $props();

	// Room switcher data rides the layout load that already resolved slug->id —
	// zero extra requests. Falls back to just the open room if absent.
	const rooms = $derived((page.data as { rooms?: WorkspaceData[] }).rooms ?? [workspace]);
	const switchable = $derived(rooms.length > 1);

	const overviewHref = $derived(`/workspace/${workspace.slug}`);
	const documentsHref = $derived(`/workspace/${workspace.slug}/document`);
	const accessManagementHref = $derived(`/workspace/${workspace.slug}/management-access`);
	const qaHref = $derived(resolve('/(app)/workspace/[slug]/qa', { slug: workspace.slug }));
	const activityHref = $derived(
		resolve('/(app)/workspace/[slug]/activity', { slug: workspace.slug })
	);
	const trashHref = $derived(resolve('/(app)/workspace/[slug]/trash', { slug: workspace.slug }));
	const isActive = (href: string) => page.url.pathname === href;
	// Active for the module's own route and any of its sub-routes.
	const isSection = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
	// Members/roles/groups admin surface — managers only (owner/admin).
	const showAccess = $derived(!!access && canManageAccess(access.role));
	// Q&A off for the guest's group = section hidden entirely; managers always see it.
	const showQa = $derived(qaEnabled || showAccess);
	// A dated membership shows its date where the member sees the room: losing
	// access without any signal is the failure this one line prevents.
	const accessUntil = $derived(access?.expires_at ? formatDateLocal(access.expires_at) : '');
	const logoSrc = $derived(workspaceLogoUrl(workspace));
</script>

<nav class="flex h-full flex-col gap-1 p-3" aria-label="Navigasi ruang data">
	<!-- Context header: 1-click exit to the rooms list + which room is open. -->
	<a
		href="/workspace"
		class="inline-flex items-center gap-1.5 px-1 py-1 text-xs font-medium text-muted transition-colors hover:text-base-content"
	>
		<svg
			class="h-3.5 w-3.5 flex-none"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.8"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="m15 6-6 6 6 6" />
		</svg>
		{t('ws.detail.back')}
	</a>

	<!-- Identity block doubles as the room switcher when the member is in more
	     than one room. A shortcut, not a replacement: the back link above still
	     leads to the full list. -->
	{#if switchable}
		<div class="dropdown mt-1 mb-2 w-full">
			<button
				tabindex="0"
				class="flex w-full items-center gap-2.5 rounded-field px-1 py-1 text-left transition-colors hover:bg-base-content/5"
				aria-label={t('ws.switcher.open')}
			>
				{#if logoSrc}
					<img
						src={logoSrc}
						alt=""
						class="h-6 w-6 flex-none rounded-field border border-base-content/10 bg-base-100 object-contain"
					/>
				{:else}
					<span
						class="grid h-6 w-6 flex-none place-items-center rounded-field bg-primary/10 text-xs font-semibold text-primary"
						>{workspace.name.charAt(0).toUpperCase()}</span
					>
				{/if}
				<div class="min-w-0 flex-1">
					<span class="block truncate text-sm font-semibold tracking-[-0.01em]"
						>{workspace.name}</span
					>
					<WorkspaceStatusBadge status={workspace.status} class="mt-0.5" />
				</div>
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
					<path d="m7 15 5 5 5-5M7 9l5-5 5 5" />
				</svg>
			</button>
			<ul
				class="dropdown-content z-dropdown mt-1 max-h-80 w-full min-w-52 overflow-y-auto rounded-box border border-base-content/10 bg-base-100 p-1.5 shadow-lg"
			>
				{#each rooms as room (room.id)}
					{@const current = room.id === workspace.id}
					<li>
						<!-- eslint-disable svelte/no-navigation-without-resolve -- slug interpolation; same pattern as the nav links above -->
						<a
							href="/workspace/{room.slug}"
							class="flex items-center gap-2 rounded-field px-2 py-1.5 text-sm transition-colors {current
								? 'bg-primary/10 font-medium text-primary'
								: 'hover:bg-base-content/5'}"
							aria-current={current ? 'true' : undefined}
						>
							<span class="min-w-0 flex-1 truncate">{room.name}</span>
							<WorkspaceStatusBadge status={room.status} class="flex-none" />
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</li>
				{/each}
			</ul>
		</div>
	{:else}
		<div class="mt-1 mb-2 flex items-center gap-2.5 px-1">
			{#if logoSrc}
				<img
					src={logoSrc}
					alt=""
					class="h-6 w-6 flex-none rounded-field border border-base-content/10 bg-base-100 object-contain"
				/>
			{:else}
				<span
					class="grid h-6 w-6 flex-none place-items-center rounded-field bg-primary/10 text-xs font-semibold text-primary"
					>{workspace.name.charAt(0).toUpperCase()}</span
				>
			{/if}
			<div class="min-w-0">
				<span class="block truncate text-sm font-semibold tracking-[-0.01em]">{workspace.name}</span
				>
				<WorkspaceStatusBadge status={workspace.status} class="mt-0.5" />
			</div>
		</div>
	{/if}

	{#if accessUntil}
		<p class="mb-2 px-1 text-[0.6875rem] leading-snug text-muted">
			{t('ws.access.until')} <span class="font-mono">{accessUntil}</span>
		</p>
	{/if}

	<div class="mb-1 border-t border-base-content/10"></div>

	<!-- Overview — the only live module today. -->
	<a
		href={overviewHref}
		class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isActive(
			overviewHref
		)
			? 'bg-primary/10 text-primary'
			: 'text-base-content hover:bg-base-content/5'}"
		aria-current={isActive(overviewHref) ? 'page' : undefined}
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
			<rect x="3" y="3" width="7" height="9" rx="1.5" />
			<rect x="14" y="3" width="7" height="5" rx="1.5" />
			<rect x="14" y="12" width="7" height="9" rx="1.5" />
			<rect x="3" y="16" width="7" height="5" rx="1.5" />
		</svg>
		{t('ws.section.overview')}
	</a>

	<!-- Documents — folder index for the data room. -->
	<a
		href={documentsHref}
		class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isSection(
			documentsHref
		)
			? 'bg-primary/10 text-primary'
			: 'text-base-content hover:bg-base-content/5'}"
		aria-current={isSection(documentsHref) ? 'page' : undefined}
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
			<path d="M14 3v4a1 1 0 0 0 1 1h4" />
			<path d="M5 3h9l5 5v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z" />
		</svg>
		<span class="flex-1 text-left">{t('ws.section.documents')}</span>
	</a>

	<!-- Q&A — guests ask, managers answer. Hidden for guests whose group has
	     Q&A off. Badge = waiting count, only fed for managers by the layout load. -->
	{#if showQa}
		<a
			href={qaHref}
			class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isSection(
				qaHref
			)
				? 'bg-primary/10 text-primary'
				: 'text-base-content hover:bg-base-content/5'}"
			aria-current={isSection(qaHref) ? 'page' : undefined}
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
				<path d="M21 12a8 8 0 0 1-8 8H5a2 2 0 0 1-2-2v-6a8 8 0 1 1 18 0z" />
				<path
					d="M9.5 10.5c0-1.4 1.1-2.5 2.5-2.5s2.5 1.1 2.5 2.5c0 1.2-.9 1.8-1.7 2.3-.5.3-.8.7-.8 1.2"
				/>
				<path d="M12 16.5h.01" />
			</svg>
			<span class="flex-1 text-left">{t('ws.section.qa')}</span>
			{#if qaWaiting > 0}
				<span
					class="grid h-5 min-w-5 place-items-center rounded-full bg-primary px-1.5 text-[0.6875rem] font-semibold text-primary-content tabular-nums"
					aria-label={t('qa.waitingCount', { n: qaWaiting })}
				>
					{qaWaiting > 99 ? '99+' : qaWaiting}
				</span>
			{/if}
		</a>
	{/if}

	<!-- Activity trail, people & permissions — managers only. -->
	{#if showAccess}
		<a
			href={activityHref}
			class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isSection(
				activityHref
			)
				? 'bg-primary/10 text-primary'
				: 'text-base-content hover:bg-base-content/5'}"
			aria-current={isSection(activityHref) ? 'page' : undefined}
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
				<path d="M3 12h4l2 6 4-12 2 6h6" />
			</svg>
			<span class="flex-1 text-left">{t('ws.section.activity')}</span>
		</a>

		<a
			href={accessManagementHref}
			class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isActive(
				accessManagementHref
			)
				? 'bg-primary/10 text-primary'
				: 'text-base-content hover:bg-base-content/5'}"
			aria-current={isActive(accessManagementHref) ? 'page' : undefined}
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
				<circle cx="9" cy="8" r="3" />
				<path d="M3 20a6 6 0 0 1 12 0" />
				<path d="M16 5.5a3 3 0 0 1 0 5.5" />
				<path d="M18 13.5a6 6 0 0 1 3 5.5" />
			</svg>
			<span class="flex-1 text-left">{t('ws.section.access')}</span>
		</a>

		<a
			href={trashHref}
			class="flex items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium transition-colors {isSection(
				trashHref
			)
				? 'bg-primary/10 text-primary'
				: 'text-base-content hover:bg-base-content/5'}"
			aria-current={isSection(trashHref) ? 'page' : undefined}
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
				<path d="M3 6h18" />
				<path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
				<path d="M6 6v14a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V6" />
				<path d="M10 11v6M14 11v6" />
			</svg>
			<span class="flex-1 text-left">{t('ws.section.trash')}</span>
		</a>
	{/if}
</nav>
