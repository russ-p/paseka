<script lang="ts">
	import Section from '$lib/components/Section.svelte';

	let {
		title = 'Trail artifacts',
		note,
		class: extra,
		collapsible = false,
		open = true,
		ontoggle
	}: {
		title?: string;
		note?: string;
		class?: string;
		collapsible?: boolean;
		open?: boolean;
		ontoggle?: (open: boolean) => void;
	} = $props();
</script>

<Section {title} {note} class={extra} {collapsible} {open} {ontoggle}>
	<p>comb body</p>
	{#snippet actions()}
		<button type="button" class="btn btn-xs">Load more</button>
	{/snippet}
</Section>
