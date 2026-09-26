<script lang="ts">
	import { page } from '$app/state';
	import TraceDetail from './TraceDetail.svelte';

	// `page.params` is the only source of the trail id; the body lives in
	// TraceDetail so a test can mount it without a router.
	const traceId = $derived(page.params.traceId ?? '');
</script>

<TraceDetail {traceId} />
