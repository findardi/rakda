<script lang="ts">
	import { setContext } from 'svelte';
	import { page } from '$app/state';
	import { showToast } from '$lib/components/common';
	import { InviteDialog } from '$lib/components/app';
	import { isManager } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { MyAccessWorkspace } from '$lib/types/workspace';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const viewerRole = $derived((page.data as { access?: MyAccessWorkspace }).access?.role ?? '');
	const canManage = $derived(isManager(viewerRole));

	const base = $derived(`/workspace/${page.params.slug}/management-access/member`);
	const subtabs = $derived([
		{ href: base, label: t('ma.member'), count: data.members.length, exact: true },
		{ href: `${base}/invite`, label: t('ma.pending'), count: data.pendingCount, exact: false }
	]);
	const isActive = (href: string, exact: boolean) =>
		exact ? page.url.pathname === href : page.url.pathname.startsWith(href);

	// One invite entry point for the whole section; children (e.g. an empty-state
	// CTA) open it through context instead of each owning a dialog.
	let inviteOpen = $state(false);
	setContext('member-invite', { open: () => (inviteOpen = true) });
</script>

<div class="flex flex-wrap items-center justify-between gap-3">
	<!-- Segmented control: secondary nav, deliberately distinct from the underline
		 tabs above it so the two rows never read as the same control. -->
	<nav class="inline-flex gap-1 rounded-field bg-base-content/4 p-1" aria-label={t('ma.member')}>
		{#each subtabs as tab (tab.href)}
			{@const active = isActive(tab.href, tab.exact)}
			<a
				href={tab.href}
				aria-current={active ? 'page' : undefined}
				class="inline-flex items-center gap-2 rounded-selector px-3 py-1.5 text-sm font-medium transition-colors {active
					? 'bg-base-100 text-base-content shadow-sm'
					: 'text-muted hover:text-base-content'}"
			>
				{tab.label}
				<span class="font-mono text-xs {active ? 'text-primary' : 'text-muted'}" aria-hidden="true"
					>{tab.count}</span
				>
			</a>
		{/each}
	</nav>

	{#if canManage}
		<button type="button" onclick={() => (inviteOpen = true)} class="btn btn-primary btn-sm">
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
				<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
				<circle cx="9" cy="7" r="4" />
				<path d="M19 8v6M22 11h-6" />
			</svg>
			{t('member.invite')}
		</button>
	{/if}
</div>

<div class="mt-5">
	{@render children()}
</div>

<InviteDialog
	bind:open={inviteOpen}
	roles={data.roles}
	groups={data.groups}
	{viewerRole}
	action={`${base}/invite?/invite`}
	pendingHref={`${base}/invite`}
	oncompleted={(n) => n && showToast(t('member.invite.toast', { n }), 'success')}
/>
