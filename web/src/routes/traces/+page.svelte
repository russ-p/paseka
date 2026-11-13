<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import { createTracesStore, type TracesStore } from '$lib/stores/traces.svelte';
	import { traceDetailPath } from '$lib/navigation';
	import {
		formatTimestamp,
		traceBees,
		tracePendingLabel,
		tracePrimaryLabel,
		traceStateBadge,
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
			// The run count already has a column, so this only speaks when it has
			// news: a healed trail keeps the historical flag but shows no badge,
			// and one with open failures names the count.
			key: 'state',
			label: 'State',
			text: (trace) => traceStateLabel(trace),
			badge: (trace) => traceStateBadge(trace)
		},
		{
			// Energy admission is the trail's axis, so it sits beside the trail and never
			// inside a task row. The paused badge and the count travel together: a pause
			// with nothing waiting is still worth saying, and waiting work outlives a
			// partially drained queue after a top-up.
			key: 'dispatch',
			label: 'Dispatch',
			text: (trace) => tracePendingLabel(trace) ?? '',
			searchText: (trace) =>
				[trace.paused ? 'paused' : '', trace.pauseReason ?? '']
					.filter(Boolean)
					.join(' '),
			badge: (trace) => (trace.paused ? { status: 'paused', label: 'paused' } : null),
			badgeWithText: true
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
	<title>Traces · Queen Console</title>
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
		<!-- `pageSize={0}`: the depth of this list is the store's, so the table shows every
		     trail it holds and pages nothing of its own. It writes no `?page=`, which leaves
		     `?q=` as the whole of this route's shareable view. No count either — the loaded
		     window is not the history, and a number beside these rows reads as a total the
		     server cannot honestly produce. -->
		<DataTable
			label="Traces"
			{columns}
			rows={store.traces}
			rowKey={(trace) => trace.traceId}
			emptyMessage="No trails yet."
			filterLabel="Filter traces"
			filterPlaceholder="title, id, bee"
			pageSize={0}
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
