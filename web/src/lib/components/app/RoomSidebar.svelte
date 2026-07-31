<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { canManageAccess } from '$lib/access/roles';
	import WorkspaceStatusBadge from './WorkspaceStatusBadge.svelte';
	import type { MyAccessWorkspace, WorkspaceData } from '$lib/types/workspace';

	type Props = { workspace: WorkspaceData; access?: MyAccessWorkspace };
	let { workspace, access }: Props = $props();

	const overviewHref = $derived(`/workspace/${workspace.slug}`);
	const documentsHref = $derived(`/workspace/${workspace.slug}/document`);
	const accessManagementHref = $derived(`/workspace/${workspace.slug}/management-access`);
	const trashHref = $derived(resolve('/(app)/workspace/[slug]/trash', { slug: workspace.slug }));
	const isActive = (href: string) => page.url.pathname === href;
	// Active for the module's own route and any of its sub-routes.
	const isSection = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
	// Members/roles/groups admin surface — managers only (owner/admin).
	const showAccess = $derived(!!access && canManageAccess(access.role));
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

	<div class="mt-1 mb-2 flex items-center gap-2.5 px-1">
		<span
			class="grid h-6 w-6 flex-none place-items-center rounded-field bg-primary/10 text-xs font-semibold text-primary"
			>{workspace.name.charAt(0).toUpperCase()}</span
		>
		<div class="min-w-0">
			<span class="block truncate text-sm font-semibold tracking-[-0.01em]">{workspace.name}</span>
			<WorkspaceStatusBadge status={workspace.status} class="mt-0.5" />
		</div>
	</div>

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

	<!-- Activity / audit trail — not built yet. -->
	<button
		type="button"
		disabled
		title={t('app.nav.soon')}
		class="flex cursor-not-allowed items-center gap-3 rounded-field px-3 py-2 text-[0.9375rem] font-medium text-muted/70"
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
		<span class="text-[0.6875rem] font-normal text-muted">{t('app.nav.soon')}</span>
	</button>

	<!-- People & permissions — managers only. -->
	{#if showAccess}
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
