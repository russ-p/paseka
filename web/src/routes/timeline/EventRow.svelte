<script lang="ts">
	import SignalCard from '$lib/components/SignalCard.svelte';
	import type { EventFeedItem } from '$lib/api/types';

	let {
		event,
		traceHref,
		showTrace = true
	}: {
		event: EventFeedItem;
		/** Where the row's trail id leads; omitted while that route is a placeholder. */
		traceHref?: string;
		showTrace?: boolean;
	} = $props();

	/**
	 * The raw envelope, pretty-printed once per row. It is already on the wire, so
	 * the expand is local — no request, and no modal to open and close per event.
	 */
	const rawJson = $derived(JSON.stringify(event.raw, null, 2));
</script>

<SignalCard signal={event} {traceHref} {showTrace}>
	{#snippet expand()}
		<!-- Borderless on purpose: a closed box on every card is noise, and the feed is
		     the only surface with one of these per row. -->
		<details class="mt-1">
			<summary class="cursor-pointer text-xs text-base-content/40 hover:text-base-content/60">
				Raw event
			</summary>
			<pre class="mt-1 max-h-80 overflow-auto rounded-box bg-base-200/50 p-2 text-xs whitespace-pre-wrap">{rawJson}</pre>
		</details>
	{/snippet}
</SignalCard>
