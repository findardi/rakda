<script lang="ts" module>
	// Items are data, not markup, so this component owns the menu semantics:
	// every entry is a real `menuitem`, links and buttons cannot be mixed up,
	// and a separator is a separator.
	export type MenuItem =
		| {
				kind?: 'button';
				id: string;
				label: string;
				onselect: () => void;
				danger?: boolean;
				/** SVG path `d` strings, drawn with the shared stroke style. */
				icon?: string[];
				/** Right-aligned mono hint, e.g. a keyboard shortcut. */
				hint?: string;
		  }
		| { kind: 'link'; id: string; label: string; href: string; icon?: string[] }
		| { kind: 'separator' };
</script>

<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import { on } from 'svelte/events';

	type Props = {
		/** Trigger aria-label, e.g. "Tindakan lain untuk Keuangan". */
		label: string;
		items: MenuItem[];
		/** Which trigger edge the panel aligns to; a right-aligned rail wants `end`. */
		align?: 'start' | 'end';
		/** Trigger classes — the caller passes its own icon-button class. */
		class?: string;
		/** Fires on open and close so a hover-revealed host can stay visible. */
		onopenchange?: (open: boolean) => void;
		/** Trigger glyph; defaults to three dots. */
		children?: Snippet;
	};

	let {
		label,
		items,
		align = 'end',
		class: triggerClass = '',
		onopenchange,
		children
	}: Props = $props();

	// The panel is a native popover: it renders in the top layer, so a scrolling
	// rail cannot clip it and z-index is moot, and outside-click plus Escape
	// dismissal come from the platform. Only placement and roving focus are ours.
	const panelId = $props.id();
	let trigger = $state<HTMLButtonElement>();
	let panel = $state<HTMLDivElement>();
	let open = $state(false);

	function menuItems(): HTMLElement[] {
		return Array.from(panel?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
	}

	// Measured after showPopover(): a hidden popover has no box. Below the
	// trigger, flipped above when the viewport runs out, clamped sideways.
	function place() {
		if (!trigger || !panel) return;
		const r = trigger.getBoundingClientRect();
		const w = panel.offsetWidth;
		const h = panel.offsetHeight;
		const gap = 4;
		const margin = 8;

		let top = r.bottom + gap;
		if (top + h > window.innerHeight - margin) top = Math.max(margin, r.top - gap - h);

		let left = align === 'end' ? r.right - w : r.left;
		left = Math.min(Math.max(margin, left), window.innerWidth - w - margin);

		panel.style.top = `${top}px`;
		panel.style.left = `${left}px`;
	}

	async function show(focusLast = false) {
		if (!panel || panel.matches(':popover-open')) return;
		panel.showPopover();
		place();
		await tick();
		const list = menuItems();
		(focusLast ? list.at(-1) : list[0])?.focus();
	}

	function hide() {
		if (panel?.matches(':popover-open')) panel.hidePopover();
	}

	function toggle() {
		if (open) hide();
		else void show();
	}

	function onToggle(e: ToggleEvent) {
		const next = e.newState === 'open';
		if (open === next) return;
		open = next;
		onopenchange?.(next);
		// Light dismiss parks focus on <body>; bring it back to the trigger so a
		// keyboard user is not dropped at the top of the page.
		if (!next && document.activeElement === document.body) trigger?.focus();
	}

	// Hide → refocus → act, so an action that moves focus itself (rename opens
	// an input) wins over the refocus.
	function select(item: Extract<MenuItem, { onselect: () => void }>) {
		hide();
		trigger?.focus();
		item.onselect();
	}

	function triggerKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			void show(e.key === 'ArrowUp');
		}
	}

	function panelKeydown(e: KeyboardEvent) {
		const list = menuItems();
		if (list.length === 0) return;
		const i = list.indexOf(document.activeElement as HTMLElement);

		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				list[(i + 1) % list.length]?.focus();
				break;
			case 'ArrowUp':
				e.preventDefault();
				list[(i - 1 + list.length) % list.length]?.focus();
				break;
			case 'Home':
				e.preventDefault();
				list[0]?.focus();
				break;
			case 'End':
				e.preventDefault();
				list.at(-1)?.focus();
				break;
			case 'Tab':
				// Deterministic across browsers: close and return to the trigger.
				e.preventDefault();
				hide();
				trigger?.focus();
				break;
		}
	}

	// Focus leaving the panel closes it. A null `relatedTarget` is deliberately
	// ignored: on macOS Safari and Firefox a click on a <button> does not focus
	// it, so closing there would swallow the item's own click — pointer dismissal
	// is the popover's job, not this handler's.
	function panelFocusout(e: FocusEvent) {
		const next = e.relatedTarget as Node | null;
		if (next && !panel?.contains(next) && next !== trigger) hide();
	}

	// A fixed-positioned panel must not float away from a scrolled rail.
	$effect(() => {
		if (!open) return;
		const offScroll = on(window, 'scroll', hide, { capture: true, passive: true });
		const offResize = on(window, 'resize', hide);
		return () => {
			offScroll();
			offResize();
		};
	});

	const itemClass =
		'flex w-full items-center gap-2.5 rounded-field px-2.5 py-2 text-left text-sm no-underline focus-visible:outline-none';
</script>

<button
	bind:this={trigger}
	type="button"
	aria-haspopup="menu"
	aria-expanded={open}
	aria-controls={panelId}
	aria-label={label}
	title={label}
	class={triggerClass}
	onclick={toggle}
	onkeydown={triggerKeydown}
>
	{#if children}
		{@render children()}
	{:else}
		<svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
			<circle cx="12" cy="5" r="1.6" />
			<circle cx="12" cy="12" r="1.6" />
			<circle cx="12" cy="19" r="1.6" />
		</svg>
	{/if}
</button>

<!-- Inside a draggable row a press-and-drag on an item must not lift the row. -->
<div
	bind:this={panel}
	id={panelId}
	popover="auto"
	role="menu"
	aria-label={label}
	tabindex="-1"
	ontoggle={onToggle}
	onkeydown={panelKeydown}
	onfocusout={panelFocusout}
	ondragstart={(e) => e.preventDefault()}
	class="rakda-menu fixed inset-auto m-0 min-w-52 rounded-box border border-base-content/10 bg-base-100 p-1.5 text-base-content shadow-lg"
>
	{#each items as item, i (item.kind === 'separator' ? `sep-${i}` : item.id)}
		{#if item.kind === 'separator'}
			<div role="separator" class="my-1 border-t border-base-content/10"></div>
		{:else if item.kind === 'link'}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- href is built with resolve() by the caller -->
			<a
				role="menuitem"
				tabindex="-1"
				href={item.href}
				draggable="false"
				class="{itemClass} hover:bg-base-content/5 focus-visible:bg-base-content/5"
			>
				{@render glyph(item.icon)}
				<span class="min-w-0 flex-1 truncate">{item.label}</span>
			</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{:else}
			<button
				type="button"
				role="menuitem"
				tabindex="-1"
				onclick={() => select(item)}
				class="{itemClass} {item.danger
					? 'text-error hover:bg-error/10 focus-visible:bg-error/10'
					: 'hover:bg-base-content/5 focus-visible:bg-base-content/5'}"
			>
				{@render glyph(item.icon)}
				<span class="min-w-0 flex-1 truncate">{item.label}</span>
				{#if item.hint}
					<span class="flex-none font-mono text-[0.6875rem] text-muted">{item.hint}</span>
				{/if}
			</button>
		{/if}
	{/each}
</div>

{#snippet glyph(paths?: string[])}
	{#if paths}
		<svg
			class="h-4 w-4 flex-none"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.8"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			{#each paths as d (d)}<path {d} />{/each}
		</svg>
	{:else}
		<span class="h-4 w-4 flex-none" aria-hidden="true"></span>
	{/if}
{/snippet}

<style>
	/* A floating layer may settle in; nothing decorative, and off under reduced motion. */
	.rakda-menu:popover-open {
		animation: rakda-menu-in 150ms ease-out;
	}
	@keyframes rakda-menu-in {
		from {
			opacity: 0;
			translate: 0 -2px;
		}
		to {
			opacity: 1;
			translate: 0 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-menu:popover-open {
			animation: none;
		}
	}
</style>
