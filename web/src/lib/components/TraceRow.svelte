<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { traceMeta, tracePrimaryLabel, traceState, traceStateLabel } from '$lib/format';
	import type { TraceSummary } from '$lib/api/types';

	let {
		trace,
		href,
		selected = false
	}: {
		trace: TraceSummary;
		/** Detail destination; omitted while a route has no detail surface yet. */
		href?: string;
		selected?: boolean;
	} = $props();

	const state = $derived(traceState(trace));
	const meta = $derived(traceMeta(trace));
</script>

<li
	class="trace-row rounded-box border border-base-300 bg-base-100 p-3"
	class:border-primary={selected}
	data-trace={trace.traceId}
>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0">
			{#if href}
				<a class="link truncate font-semibold" {href}>{tracePrimaryLabel(trace)}</a>
			{:else}
				<p class="truncate font-semibold">{tracePrimaryLabel(trace)}</p>
			{/if}
			<Hint lines={[meta]}>
				<p class="truncate text-xs text-base-content/50">{meta}</p>
			</Hint>
		</div>
		<div class="flex shrink-0 items-center gap-1">
			{#if trace.standing}
				<StatusBadge status="standing" label="standing" />
			{/if}
			<StatusBadge status={state} label={traceStateLabel(trace)} />
		</div>
	</div>
	{#if trace.summary}
		<p class="mt-2 line-clamp-2 text-sm text-base-content/70">{trace.summary}</p>
	{/if}
	<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-base-content/50">
		{#if trace.title}
			<span class="font-mono">{trace.traceId}</span>
		{/if}
		{#if trace.bees?.length}
			<span>{trace.bees.join(', ')}</span>
		{/if}
	</div>
</li>
