<script lang="ts">
	import { base } from '$app/paths';
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import SignalCard from '$lib/components/SignalCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import {
		runEventSummary,
		runIdentityRows,
		runStateLabel,
		runUsageRows
	} from '$lib/format';
	import { consolePath, runDetailPath, traceDetailPath } from '$lib/navigation';
	import { createRunStore, type RunStore } from '$lib/stores/run.svelte';

	let { store = createRunStore(), traceId = '', agentId = '' }: { store?: RunStore; traceId?: string; agentId?: string } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	$effect(() => {
		if (traceId && agentId) void store.select(traceId, agentId);
	});

	const run = $derived(store.current);
	const position = $derived(store.position);
	const identityRows = $derived(runIdentityRows(run, (id) => traceDetailPath(base, id)));
	const usageRows = $derived(runUsageRows(run?.usage));
	const events = $derived(store.events);

	/** `1 of 5`, or nothing when this run is the only one its trail recorded. */
	const positionLabel = $derived(
		position.total > 1 && position.index >= 0 ? `${position.index + 1} of ${position.total}` : ''
	);
</script>

<svelte:head>
	<title>Run {agentId || ''} · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<a class="link text-xs text-base-content/60" href={consolePath(base, '/runs')}>← Runs</a>
			<div class="flex flex-wrap items-center gap-2">
				<h1 class="font-mono text-2xl font-bold">{agentId || 'Run'}</h1>
				{#if run}
					<StatusBadge status={run.state} label={runStateLabel(run.state)} />
				{/if}
			</div>
			{#if run}
				<p class="text-base-content/70">
					{run.bee} on {run.adapter}
					{#if run.taskId}
						for task <span class="font-mono">{run.taskId}</span>
					{/if}
				</p>
			{/if}
		</div>

		<!-- Stepping through a trail's runs in order is how an operator reads what a
		     bee did across attempts, so it is a control rather than a list position. -->
		<div class="flex items-center gap-2">
			{#if positionLabel}
				<span id="run-position" class="text-xs text-base-content/50">{positionLabel} in this trail</span>
			{/if}
			<!-- Real links, not buttons that swap the store: a run has its own URL, so
			     stepping to a sibling must put that URL in the address bar or a
			     bookmarked run cannot be shared and Back does the wrong thing. -->
			{#if position.previous}
				<a
					id="run-previous"
					class="btn btn-sm"
					aria-label="Previous run in this trail"
					href={runDetailPath(base, position.previous.traceId, position.previous.agentId)}
				>
					<ChevronLeft class="h-4 w-4" strokeWidth={2.5} />
				</a>
			{:else}
				<button
					id="run-previous"
					type="button"
					class="btn btn-sm"
					aria-label="Previous run in this trail"
					disabled
				>
					<ChevronLeft class="h-4 w-4" strokeWidth={2.5} />
				</button>
			{/if}
			{#if position.next}
				<a
					id="run-next"
					class="btn btn-sm"
					aria-label="Next run in this trail"
					href={runDetailPath(base, position.next.traceId, position.next.agentId)}
				>
					<ChevronRight class="h-4 w-4" strokeWidth={2.5} />
				</a>
			{:else}
				<button
					id="run-next"
					type="button"
					class="btn btn-sm"
					aria-label="Next run in this trail"
					disabled
				>
					<ChevronRight class="h-4 w-4" strokeWidth={2.5} />
				</button>
			{/if}
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if run}
		<Section id="run-identity" title="Run">
			<MetaList rows={identityRows} label="Run identity" columns={2} />
		</Section>

		{#if usageRows.length > 0}
			<Section id="run-usage" title="Usage">
				<MetaList rows={usageRows} label="Token spend" columns={2} />
			</Section>
		{/if}

		<!-- A run summary is whatever the adapter wrote, terminal escapes and all, so
		     it stays literal text in a scroll box rather than being treated as prose. -->
		{#if run.summary}
			<Section
				id="run-summary"
				title="Summary"
				note="as the adapter wrote it"
				collapsible
				open={false}
			>
				<pre class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{run.summary}</pre>
			</Section>
		{/if}

		{#if run.body}
			<!-- Folded: the body is the literal task text, often long, and reading a run
			     is about what it did rather than what it was handed. `Section` opens by
			     default, so a folded block says so out loud. -->
			<Section
				id="run-body"
				title="Task body"
				note="what the adapter was handed"
				collapsible
				open={false}
			>
				<pre class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{run.body}</pre>
			</Section>
		{/if}

		<Section
			id="run-events"
			title="Events"
			note={events.length > 0 ? `${events.length} recorded` : undefined}
		>
			{#if store.eventsError}
				<div class="alert alert-error mb-3" role="alert"><span>{store.eventsError}</span></div>
			{/if}
			{#if store.eventsLoading && events.length === 0}
				<div class="space-y-2" aria-busy="true">
					<span class="skeleton block h-16 w-full"></span>
					<span class="skeleton block h-16 w-full"></span>
				</div>
			{:else if events.length === 0}
				<p class="text-base-content/70">
					This run recorded no events. The events a run emits are what it announced on the bus,
					not its transcript — the transcript is the task body above.
				</p>
			{:else}
				<ul class="space-y-2" aria-label="Run events">
					{#each events as event (event.seq)}
						<SignalCard
							signal={runEventSummary(event)}
							showTrace={false}
							traceHref={traceDetailPath(base, event.traceId)}
						>
							{#snippet expand()}
								<details class="mt-1">
									<summary class="cursor-pointer text-xs text-base-content/40 hover:text-base-content/60">
										Raw event #{event.seq}
									</summary>
									<pre class="mt-1 max-h-80 overflow-auto rounded-box bg-base-200/50 p-2 text-xs whitespace-pre-wrap">{JSON.stringify(event, null, 2)}</pre>
								</details>
							{/snippet}
						</SignalCard>
					{/each}
				</ul>
			{/if}
		</Section>
	{:else if !store.showSkeletons}
		<div class="card bg-base-100 shadow-sm">
			<div class="card-body items-center py-16 text-center">
				<h2 class="card-title text-xl">Run not found</h2>
				<p class="text-base-content/70">
					No run <span class="font-mono">{agentId}</span> on trail
					<span class="font-mono">{traceId}</span>. It may be older than the recent window, or the
					ids may be mistyped.
				</p>
				<a class="btn btn-sm mt-2" href={consolePath(base, '/runs')}>All runs</a>
			</div>
		</div>
	{/if}
</div>
