<script lang="ts">
	type Props = { size?: number };
	let { size = 28 }: Props = $props();
</script>

<span class="inline-flex items-center gap-2 text-primary">
	<svg
		viewBox="0 0 24 24"
		fill="none"
		aria-hidden="true"
		style="width: {size}px; height: {size}px"
		class="flex-none"
	>
		<rect x="3" y="3" width="18" height="18" rx="4" stroke="currentColor" stroke-width="1.4" />
		<path
			d="M6 6.75h12M6 10.1h8.2M6 13.45h8.2M6 16.8h8.2"
			stroke="currentColor"
			stroke-width="1.4"
			stroke-linecap="round"
		/>
	</svg>
	<span class="text-[1.2rem] font-semibold tracking-[-0.01em] text-base-content">Rakda</span>
</span>
