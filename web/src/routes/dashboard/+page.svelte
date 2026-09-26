<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import CueRunModal from '$lib/components/CueRunModal.svelte';
	import SignalCard from '$lib/components/SignalCard.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import TraceRow from '$lib/components/TraceRow.svelte';
	import {
		dashboardFailedRuns,
		runStateLabel,
		taskCountEntries
	} from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import {
		consoleStatusStore,
		type ConsoleStatusStore
	} from '$lib/stores/console-status.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import type { RunSummary, RunCueResult } from '$lib/api/types';

	let {
		store = consoleStatusStore,
		toasts = toastStore
	}: { store?: ConsoleStatusStore; toasts?: ToastStore } = $props();

	let cueOpen = $state(false);

	const dashboard = $derived(store.dashboard);
	const failedRuns = $derived(dashboardFailedRuns(dashboard));
	const taskCounts = $derived(taskCountEntries(dashboard?.taskCounts));

	const failedRunColumns: DataColumn<RunSummary>[] = [
		{ key: 'bee', label: 'Bee', text: (run) => run.bee || '—' },
		{ key: 'state', label: 'State', text: (run) => runStateLabel(run.state) },
		{ key: 'trace', label: 'Trace', text: (run) => run.traceId, secondary: true },
		{ key: 'agent', label: 'Agent', text: (run) => run.agentId, secondary: true }
	];

	function cueRan(result: RunCueResult): void {
		toasts.push('success', `Cue published — trace ${result.traceId}`);
		void store.refresh();
	}
</script>

<svelte:head>
	<title>Dashboard · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-end justify-between gap-3">
		<div class="space-y-1">
			<h1 class="text-3xl font-bold">Dashboard</h1>
			<p class="text-base-content/70">
				Colony snapshot for {store.colony ?? 'this colony'}, refreshed by the dashboard poll.
			</p>
		</div>
		<button
			id="dashboard-run-cue"
			type="button"
			class="btn btn-primary"
			onclick={() => (cueOpen = true)}
		>
			Run cue
		</button>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	<section
		id="dashboard-stats"
		class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5"
	>
		{#if store.loading && !dashboard}
			{#each Array.from({ length: 5 }) as _, index (index)}
				<div class="rounded-box border border-base-300 bg-base-100 px-3 py-2">
					<span class="skeleton block h-3 w-20"></span>
					<span class="skeleton mt-1 block h-5 w-16"></span>
				</div>
			{/each}
		{:else}
			<StatTile
				label="Active traces"
				value={store.activeTraceCount}
				hint="Trails with a run still in flight."
			/>
			<StatTile
				label="Active sessions"
				value={dashboard?.activeSessions ?? 0}
				hint="Interactive HITL sessions currently attached."
			/>
			<StatTile
				label="Active worktrees"
				value={dashboard?.activeWorktrees ?? 0}
				hint="Isolated checkouts under .paseka/worktrees."
			/>
			<StatTile
				id="dashboard-task-counts"
				label="Task counts"
				hint="Tasks across the recent traces, by status."
				class="lg:col-span-2"
			>
				{#if taskCounts.length === 0}
					<p class="text-sm text-base-content/60">None</p>
				{:else}
					<ul class="flex flex-wrap items-center gap-x-3 gap-y-1">
						{#each taskCounts as [status, count] (status)}
							<li class="flex items-center gap-1.5">
								<span class="text-sm text-base-content/70">{status}</span>
								<StatusBadge status={status} label={String(count)} />
							</li>
						{/each}
					</ul>
				{/if}
			</StatTile>
		{/if}
	</section>

	<section id="dashboard-traces" class="space-y-3">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h2 class="text-xl font-semibold">Recent traces</h2>
			<a class="link text-sm" href="/next/traces">All traces</a>
		</div>
		{#if store.loading && !dashboard}
			<ul class="space-y-2">
				{#each Array.from({ length: 3 }) as _, index (index)}
					<li class="rounded-box border border-base-300 bg-base-100 p-3">
						<span class="skeleton block h-4 w-2/3"></span>
						<span class="skeleton mt-2 block h-3 w-1/3"></span>
					</li>
				{/each}
			</ul>
		{:else if (dashboard?.recentTraces.length ?? 0) === 0}
			<p class="rounded-box border border-base-300 bg-base-100 p-4 text-sm text-base-content/60">
				No recent traces.
			</p>
		{:else}
			<ul class="space-y-2">
				{#each dashboard?.recentTraces ?? [] as trace (trace.traceId)}
					<TraceRow {trace} href={traceDetailPath(base, trace.traceId)} />
				{/each}
			</ul>
		{/if}
	</section>

	<section id="dashboard-failed-runs" class="space-y-3">
		<h2 class="text-xl font-semibold">Failed runs</h2>
		<DataTable
			label="Failed runs"
			columns={failedRunColumns}
			rows={failedRuns}
			rowKey={(run) => `${run.traceId}/${run.agentId}`}
			emptyMessage="No failed runs."
			filterLabel="Filter failed runs"
			filterPlaceholder="bee, state, trace"
			pageSize={10}
		/>
	</section>

	<section id="dashboard-insights" class="space-y-3">
		<h2 class="text-xl font-semibold">Recent insights</h2>
		{#if (dashboard?.recentInsights.length ?? 0) === 0}
			<p class="rounded-box border border-base-300 bg-base-100 p-4 text-sm text-base-content/60">
				No recent insights.
			</p>
		{:else}
			<ul class="space-y-2">
				{#each dashboard?.recentInsights ?? [] as insight (`${insight.traceId}/${insight.agentId}/${insight.createdAt}`)}
					<SignalCard signal={insight} />
				{/each}
			</ul>
		{/if}
	</section>
</div>

<CueRunModal
	open={cueOpen}
	onclose={() => (cueOpen = false)}
	onran={cueRan}
/>
