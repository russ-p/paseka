<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { formatTimestamp } from '$lib/format';
	import type { SignalSummary } from '$lib/api/types';

	let {
		signal,
		href,
		showTrace = true
	}: {
		signal: SignalSummary;
		/** Detail destination; omitted while the feed has no target surface yet. */
		href?: string;
		/** Inside a trace the id is in the page header, so it can be dropped. */
		showTrace?: boolean;
	} = $props();

	const time = $derived(formatTimestamp(signal.createdAt));

	/** A feed row names both the contract and the payload kind; the dashboard's insight projection carries one word. */
	const kindLine = $derived(
		signal.type && signal.payloadKind && signal.type !== signal.payloadKind
			? `${signal.type} · ${signal.payloadKind}`
			: (signal.payloadKind ?? signal.type ?? '')
	);
</script>

<li class="signal-card rounded-box border border-base-300 bg-base-100 p-3" data-trace={signal.traceId}>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0">
			{#if href}
				<a class="link line-clamp-2 text-sm" {href}>{signal.summary}</a>
			{:else}
				<p class="line-clamp-2 text-sm">{signal.summary}</p>
			{/if}
		</div>
		{#if signal.severity}
			<StatusBadge status={signal.severity} label={signal.severity} />
		{/if}
	</div>
	<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-base-content/50">
		<Hint lines={[kindLine, signal.bee ?? 'unknown bee', time]}>
			<span class="cursor-help">
				{kindLine}{signal.bee ? ` · ${signal.bee}` : ''} · {time}
			</span>
		</Hint>
		{#if showTrace}
			<span class="font-mono">{signal.traceId}</span>
		{/if}
		{#if signal.agentId}
			<span class="font-mono">{signal.agentId}</span>
		{/if}
	</div>
</li>
