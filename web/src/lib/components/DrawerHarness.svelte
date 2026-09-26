<script lang="ts">
	import Drawer from '$lib/components/Drawer.svelte';
	import type { Snippet } from 'svelte';

	let {
		open = false,
		title = 'New task',
		children,
		footer
	}: {
		open?: boolean;
		title?: string;
		children?: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<Drawer {open} {title} onclose={() => (open = false)}>
	{#if children}{@render children()}{/if}
	<p>drawer body</p>
	{#if footer}
		{#snippet footer()}
			{@render footer()}
		{/snippet}
	{/if}
</Drawer>
