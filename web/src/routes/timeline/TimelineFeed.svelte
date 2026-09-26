<script lang="ts">
	import { untrack } from 'svelte';
	import { base } from '$app/paths';
	import Section from '$lib/components/Section.svelte';
	import EventRow from './EventRow.svelte';
	import { activeFilterCount, eventFilterSummary } from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import { createTimelineStore, type TimelineStore } from '$lib/stores/timeline.svelte';
	import { eventTypes, type EventFilters } from '$lib/api/types';

	let {
		store,
		initialFilters = {}
	}: { store?: TimelineStore; initialFilters?: EventFilters } = $props();

	// The default store is seeded from the same deep link the form is, so the first
	// read is already scoped. A default-prop expression cannot see `initialFilters`
	// from its own destructure, hence the local. The untrack is deliberate: a deep
	// link from a trail detail remounts this page, so the seed is a one-time read
	// and re-reading it would re-scope the feed under the operator's hands.
	const seed = untrack(() => ({ ...initialFilters }));
	const ownStore = createTimelineStore({ initialFilters: seed });
	const timeline = $derived(store ?? ownStore);

	let filters = $state<EventFilters>(seed);

	$effect(() => {
		timeline.start();
	});

	const active = $derived(timeline.filters);
	const applied = $derived(activeFilterCount(active));
	const summary = $derived(eventFilterSummary(active));
	/** Scoped to one trail, so the feed rows need not repeat the id they all share. */
	const scopedToTrace = $derived(Boolean(active.traceId));

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await timeline.apply(filters);
	}

	async function clearFilters(): Promise<void> {
		filters = {};
		await timeline.clear();
	}
</script>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<h1 class="text-3xl font-bold">Timeline</h1>
			<p class="text-base-content/70">
				Every event the colony recorded, newest first — the four contracts and the work each one
				announced.
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			{#if applied > 0}
				<button id="timeline-clear" type="button" class="btn btn-sm" onclick={() => void clearFilters()}>
					Clear {applied} filter{applied === 1 ? '' : 's'}
				</button>
			{/if}
			<button
				id="timeline-refresh"
				type="button"
				class="btn btn-sm"
				disabled={timeline.busy}
				onclick={() => void timeline.refresh()}
			>
				{timeline.loading ? 'Refreshing…' : 'Refresh'}
			</button>
		</div>
	</header>

	<!-- Six filters is a wall of chrome above a feed, so the panel is folded unless
	     something is in it, and the summary line then says what. -->
	<Section
		id="timeline-filters"
		title="Filters"
		note={applied > 0 ? summary : undefined}
		collapsible
		open={applied > 0}
	>
		<form id="timeline-filter-form" class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3" onsubmit={submit}>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Trace</span>
				<input
					id="timeline-filter-trace"
					type="text"
					class="input input-sm w-full"
					placeholder="trace id"
					bind:value={filters.traceId}
				/>
			</label>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Task</span>
				<input
					id="timeline-filter-task"
					type="text"
					class="input input-sm w-full"
					placeholder="task id"
					bind:value={filters.taskId}
				/>
			</label>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Bee</span>
				<input
					id="timeline-filter-bee"
					type="text"
					class="input input-sm w-full"
					placeholder="bee role"
					bind:value={filters.bee}
				/>
			</label>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Type</span>
				<select id="timeline-filter-type" class="select select-sm w-full" bind:value={filters.type}>
					<option value="">Any contract</option>
					{#each eventTypes as type (type)}
						<option value={type}>{type}</option>
					{/each}
				</select>
			</label>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Kind</span>
				<input
					id="timeline-filter-kind"
					type="text"
					class="input input-sm w-full"
					placeholder="payload.kind"
					bind:value={filters.kind}
				/>
			</label>
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Severity</span>
				<input
					id="timeline-filter-severity"
					type="text"
					class="input input-sm w-full"
					placeholder="severity"
					bind:value={filters.severity}
				/>
			</label>
			<div class="flex items-end gap-2 sm:col-span-2 lg:col-span-3">
				<button id="timeline-apply" type="submit" class="btn btn-primary btn-sm" disabled={timeline.busy}>
					Apply
				</button>
				{#if applied > 0}
					<button id="timeline-reset" type="button" class="btn btn-ghost btn-sm" onclick={() => void clearFilters()}>
						Reset
					</button>
				{/if}
			</div>
		</form>
	</Section>

	{#if timeline.lastError}
		<div class="alert alert-error" role="alert"><span>{timeline.lastError}</span></div>
	{/if}

	{#if timeline.showSkeletons}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-16 w-full"></span>
			<span class="skeleton block h-16 w-full"></span>
			<span class="skeleton block h-16 w-full"></span>
		</div>
	{:else}
		{#if timeline.items.length === 0}
			<div class="card bg-base-100 shadow-sm">
				<div class="card-body items-center py-14 text-center">
					<h2 class="card-title text-xl">No events</h2>
					<p class="text-base-content/70">
						{#if applied > 0}
							Nothing matches {summary}. Widen the filters to see the rest of the colony.
						{:else}
							The colony has not recorded any events yet.
						{/if}
					</p>
				</div>
			</div>
		{:else}
			<ul id="timeline-feed" class="space-y-2" aria-label="Event feed">
				{#each timeline.items as event (event.id)}
					<EventRow
						{event}
						showTrace={!scopedToTrace}
						traceHref={scopedToTrace ? undefined : traceDetailPath(base, event.traceId)}
					/>
				{/each}
			</ul>
			{#if timeline.hasMore}
				<div class="flex justify-center pt-1">
					<button
						id="timeline-load-more"
						type="button"
						class="btn btn-sm"
						disabled={timeline.loadingMore}
						onclick={() => void timeline.loadMore()}
					>
						{timeline.loadingMore ? 'Loading…' : 'Load more'}
					</button>
				</div>
			{/if}
		{/if}
	{/if}
</div>
