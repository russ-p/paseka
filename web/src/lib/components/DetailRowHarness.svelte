<script lang="ts">
	import DetailRow from '$lib/components/DetailRow.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';

	let {
		title = 'Prune implementation notes',
		meta,
		detail,
		href,
		dataKey
	}: {
		title?: string;
		meta?: string;
		detail?: string;
		href?: string;
		dataKey?: string;
	} = $props();
</script>

<DetailRow {title} {meta} {detail} {href} {dataKey}>
	{#snippet side()}
		<StatusBadge status="announced" label="announced" />
	{/snippet}
</DetailRow>
