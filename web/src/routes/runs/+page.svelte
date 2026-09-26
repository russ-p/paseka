<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import { runStateLabel, formatTimestamp } from '$lib/format';
	import { runDetailPath, traceDetailPath } from '$lib/navigation';
	import { createRunStore, type RunStore } from '$lib/stores/run.svelte';
	import type { RunSummary } from '$lib/api/types';

	let { store = createRunStore() }: { store?: RunStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const runColumns: DataColumn<RunSummary>[] = [
		{
			key: 'started',
			label: 'Started',
			// The row's identity is when it began, which is also what a run list is
			// scanned by; a run id is meaningless without the trail beside it.
			text: (run) => formatTimestamp(run.startedAt) ?? '—',
			searchText: (run) => run.startedAt
		},
		{ key: 'bee', label: 'Bee', text: (run) => run.bee, mono: true, searchText: (run) => run.adapter },
		{
			key: 'state',
			label: 'State',
			// One word that decides what happens next: a queued or running run is being
			// watched, a failed one wants reading, a completed one is done.
			text: (run) => runStateLabel(run.state),
			badge: (run) => ({ status: run.state, label: runStateLabel(run.state) })
		},
		{
			key: 'trace',
			label: 'Trail',
			text: (run) => run.traceId,
			mono: true,
			href: (run) => traceDetailPath(base, run.traceId),
			// A trail id is 24 characters and a run id 14, so without this the widest
			// column pushes the run's own link off the right edge, where the table's
			// `overflow-x-hidden` would clip it rather than let it scroll.
			grow: true
		},
		{ key: 'task', label: 'Task', text: (run) => run.taskId ?? '', mono: true, secondary: true },
		{
			key: 'run',
			label: 'Run',
			text: (run) => run.agentId,
			mono: true,
			href: (run) => runDetailPath(base, run.traceId, run.agentId)
		}
	];
</script>

<svelte:head>
	<title>Runs · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-1">
		<h1 class="text-3xl font-bold">Runs</h1>
		<p class="text-base-content/70">
			Every headless adapter run the colony recorded, newest first — what each bee was asked, which
			adapter ran it, and what it spent. Open one for its events.
		</p>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else}
		<DataTable
			label="Runs"
			columns={runColumns}
			rows={store.runs}
			rowKey={(run) => `${run.traceId}/${run.agentId}`}
			emptyMessage="No runs recorded yet."
			filterLabel="Filter runs"
			filterPlaceholder="bee, adapter, trail, task, run"
			pageSize={20}
		/>
		<p class="text-xs text-base-content/50">
			The server returns the most recent runs only; a trail older than that window is still
			reachable from its own page.
		</p>
	{/if}
</div>
