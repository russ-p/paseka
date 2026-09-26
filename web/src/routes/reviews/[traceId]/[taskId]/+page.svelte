<script lang="ts">
	import { page } from '$app/state';
	import ReviewDetail from './ReviewDetail.svelte';

	// A proposal is addressed by both ids, so `page.params` is the only source; the
	// body lives in ReviewDetail so a test can mount it without a router.
	const traceId = $derived(page.params.traceId ?? '');
	const taskId = $derived(page.params.taskId ?? '');
</script>

<ReviewDetail {traceId} {taskId} />
