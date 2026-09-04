<script lang="ts">
	import Alert from './Alert.svelte';
	import { store } from './toast.svelte';
</script>

{#if store.current}
	<div class="pointer-events-none fixed inset-x-0 bottom-4 z-toast flex justify-center px-4">
		<div class="pointer-events-auto">
			<Alert variant={store.current.variant}>
				{store.current.message}
				{#if store.current.action}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href is built with resolve() by the caller -->
					<a href={store.current.action.href} class="ml-2 font-medium underline underline-offset-2">
						{store.current.action.label}
					</a>
				{/if}
			</Alert>
		</div>
	</div>
{/if}
