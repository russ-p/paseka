<script lang="ts">
	import ArtifactViewModal from '$lib/components/ArtifactViewModal.svelte';
	import type { ArtifactView } from '$lib/api/types';

	let { first, second }: { first: ArtifactView; second: ArtifactView } = $props();

	let viewing = $state<ArtifactView | null>(null);

	$effect(() => {
		viewing = first;
	});
</script>

<button type="button" onclick={() => (viewing = first)}>Open first</button>
<button type="button" onclick={() => (viewing = second)}>Open second</button>

<ArtifactViewModal
	open={viewing !== null}
	traceId="trace-1"
	artifact={viewing}
	onclose={() => (viewing = null)}
/>
