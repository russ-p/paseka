<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import { createTracesStore, type TracesStore } from '$lib/stores/traces.svelte';
	import { traceDetailPath } from '$lib/navigation';
	import {
		formatTimestamp,
		traceBees,
		tracePrimaryLabel,
		traceState,
		traceStateLabel
	} from '$lib/format';
	import type { TraceSummary } from '$lib/api/types';

	let { store = createTracesStore() }: { store?: TracesStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const columns: DataColumn<TraceSummary>[] = [
		{
			key: 'trace',
			label: 'Trace',
			text: (trace) => tracePrimaryLabel(trace),
			// The id and the standing flag are searchable but not on screen.
			searchText: (trace) =>
				[trace.traceId, trace.standing ? 'standing' : '', trace.summary ?? '']
					.filter(Boolean)
					.join(' '),
			href: (trace) => traceDetailPath(base, trace.traceId),
			badge: (trace) => (trace.standing ? { status: 'standing', label: 'standing' } : null),
			grow: true
		},
		{
			// The run count already has a column, so this only speaks when it has news.
			key: 'state',
			label: 'State',
			text: (trace) => traceStateLabel(trace),
			badge: (trace) =>
				trace.hasActive || trace.hasFailures
					? { status: traceState(trace), label: traceState(trace) }
					: null
		},
		{ key: 'runs', label: 'Runs', text: (trace) => String(trace.runCount), align: 'right' },
		{ key: 'tasks', label: 'Tasks', text: (trace) => String(trace.taskCount), align: 'right' },
		{ key: 'bees', label: 'Bees', text: (trace) => traceBees(trace), secondary: true },
		{
			key: 'activity',
			label: 'Last activity',
			text: (trace) => formatTimestamp(trace.lastActivityAt),
			secondary: true
		}
	];
</script>

<svelte:head>
	<title>Traces · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-1">
		<h1 class="text-3xl font-bold">Traces</h1>
		<p class="text-base-content/70">
			Flight trails with their tasks, runs, and comb, newest activity first.
		</p>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	<section class="space-y-2">
		{#if !store.showSkeletons && store.traces.length > 0}
			<p class="text-xs text-base-content/50">
				Showing {store.traces.length} {store.traces.length === 1 ? 'trail' : 'trails'}, newest activity first.
			</p>
		{/if}
		<DataTable
			label="Traces"
			{columns}
			rows={store.traces}
			rowKey={(trace) => trace.traceId}
			emptyMessage="No trails yet."
			filterLabel="Filter traces"
			filterPlaceholder="title, id, bee"
			pageSize={15}
			loading={store.showSkeletons}
		/>
		{#if store.hasMore}
			<div class="flex justify-center">
				<button
					id="traces-load-more"
					type="button"
					class="btn btn-sm"
					disabled={store.loadingMore}
					onclick={() => void store.loadMore()}
				>
					{store.loadingMore ? 'Loading…' : 'Load older trails'}
				</button>
			</div>
		{/if}
	</section>
</div>
