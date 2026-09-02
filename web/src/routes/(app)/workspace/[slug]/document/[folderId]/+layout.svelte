<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { roomWritableFrom } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import type { MyAccessWorkspace } from '$lib/types/workspace';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);

	const perms = $derived((page.data as { access?: MyAccessWorkspace }).access?.permissions ?? []);
	const canAssign = $derived(perms.includes('group:assign') && roomWritableFrom(page.data));

	const docsHref = $derived(
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId })
	);
	const accessHref = $derived(
		resolve('/(app)/workspace/[slug]/document/[folderId]/access', { slug, folderId })
	);
	const onAccess = $derived(page.url.pathname === accessHref);

	const tab =
		'rounded-field px-2.5 py-1.5 text-sm no-underline transition-colors focus-visible:outline-2';
	const activeTab = 'bg-base-content/6 font-medium text-base-content';
	const idleTab = 'text-muted hover:bg-base-content/[0.045] hover:text-base-content';
</script>

{#if canAssign}
	<nav aria-label={t('doc.pane.tabs')} class="mb-3 flex items-center gap-1">
		<a
			href={docsHref}
			aria-current={onAccess ? undefined : 'page'}
			class="{tab} {onAccess ? idleTab : activeTab}"
		>
			{t('doc.pane.documents')}
		</a>
		<a
			href={accessHref}
			aria-current={onAccess ? 'page' : undefined}
			class="{tab} {onAccess ? activeTab : idleTab}"
		>
			{t('doc.pane.access')}
		</a>
	</nav>
{/if}

{@render children()}
