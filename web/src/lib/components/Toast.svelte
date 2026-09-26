<script lang="ts">
	import { toastClass, toastStore, type ToastStore } from '$lib/stores/toast.svelte';

	let { store = toastStore }: { store?: ToastStore } = $props();
</script>

<div id="toast-region" class="toast toast-end toast-bottom z-50" aria-live="polite" aria-label="Notifications">
	{#each store.items as item (item.id)}
		<div id={`toast-${item.id}`} class={`alert ${toastClass(item.tone)}`}>
			<span>{item.message}</span>
			<button
				id={`toast-${item.id}-dismiss`}
				type="button"
				class="btn btn-ghost btn-xs"
				aria-label="Dismiss notification"
				onclick={() => store.dismiss(item.id)}
			>
				✕
			</button>
		</div>
	{/each}
</div>
