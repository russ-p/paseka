<script lang="ts">
	import { page } from '$app/state';
	import RunDetail from './RunDetail.svelte';

	// `page.params` is the only source of the ids; the body lives in RunDetail so a
	// test can mount it without a router. Stepping to a sibling run changes these
	// params, which re-runs the body's own select — the same shape the trail detail
	// uses, and the reason prev/next can be plain links.
	const traceId = $derived(page.params.traceId ?? '');
	const agentId = $derived(page.params.agentId ?? '');
</script>

<RunDetail {traceId} {agentId} />
