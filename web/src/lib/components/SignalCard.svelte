<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { insightTime } from '$lib/format';
	import type { InsightHighlight } from '$lib/api/types';

	let {
		insight,
		href
	}: {
		insight: InsightHighlight;
		/** Detail destination; omitted when the feed has no target surface yet. */
		href?: string;
	} = $props();

	const time = $derived(insightTime(insight));
</script>

<li class="signal-card rounded-box border border-base-300 bg-base-100 p-3" data-trace={insight.traceId}>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0">
			{#if href}
				<a class="link line-clamp-2 text-sm" {href}>{insight.summary}</a>
			{:else}
				<p class="line-clamp-2 text-sm">{insight.summary}</p>
			{/if}
		</div>
		{#if insight.severity}
			<StatusBadge status={insight.severity} label={insight.severity} />
		{/if}
	</div>
	<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-base-content/50">
		<Hint lines={[insight.payloadKind, insight.bee ?? 'unknown bee', time]}>
			<span class="cursor-help">
				{insight.payloadKind}{insight.bee ? ` · ${insight.bee}` : ''} · {time}
			</span>
		</Hint>
		<span class="font-mono">{insight.traceId}</span>
		{#if insight.agentId}
			<span class="font-mono">{insight.agentId}</span>
		{/if}
	</div>
</li>
