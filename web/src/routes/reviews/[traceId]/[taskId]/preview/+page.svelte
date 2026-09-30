<script lang="ts">
	import { page } from '$app/state';
	import MergePreview from './MergePreview.svelte';

	const traceId = $derived(page.params.traceId ?? '');
	const taskId = $derived(page.params.taskId ?? '');
</script>

<MergePreview {traceId} {taskId} />
