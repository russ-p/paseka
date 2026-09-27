<script lang="ts">
	import { untrack } from 'svelte';
	import { base } from '$app/paths';
	import { RefreshCw } from 'lucide-svelte';
	import Section from '$lib/components/Section.svelte';
	import EventRow from './EventRow.svelte';
	import { activeFilterCount, eventFilterSummary } from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import { createTimelineStore, timelinePollIntervals, type TimelinePollSeconds, type TimelineStore } from '$lib/stores/timeline.svelte';
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
		// Untracked on purpose. This effect's job is mount and unmount, but `start` reads
		// the cadence to decide whether to arm, so tracking it would re-run the effect on
		// every choice the operator makes — and each run is a full reset read behind a
		// click that only meant to set a timer. `setPoll` re-arms instead.
		untrack(() => timeline.start());
		document.addEventListener('visibilitychange', handleVisibility);
		// Asked once as well as on the event: a tab opened in the background never fires
		// one, and it would otherwise poll a screen nobody is looking at.
		handleVisibility();
		return () => {
			document.removeEventListener('visibilitychange', handleVisibility);
			timeline.stop();
		};
	});

	const active = $derived(timeline.filters);
	const applied = $derived(activeFilterCount(active));
	const summary = $derived(eventFilterSummary(active));
	/** Scoped to one trail, so the feed rows need not repeat the id they all share. */
	const scopedToTrace = $derived(Boolean(active.traceId));
	/** The store holds the cadence; the select only names it. */
	const pollChoice = $derived(
		timeline.pollSeconds === null ? 'manual' : String(timeline.pollSeconds)
	);
	/**
	 * A feed read answers in tens of milliseconds, which is one or two frames of a
	 * spinner — long enough to look like a glitch and too short to be feedback. So the
	 * indicator is held for a second measured from the moment the read *started*, and a
	 * read landing inside another's second extends the one already on screen instead of
	 * restarting it, which is what makes a five-second cadence read as a heartbeat
	 * rather than as five flickers.
	 *
	 * The read is never delayed. This is a floor on what the operator is shown, not on
	 * what the server is asked, and `disabled` still follows the real read — the button
	 * is clickable again the moment the feed is current, even while the acknowledgement
	 * is still on screen.
	 */
	const MIN_SPIN_MS = 1000;

	let spinning = $state(false);
	let spinStartedAt = 0;
	let spinTimer: ReturnType<typeof setTimeout> | undefined;
	let settled = false;

	$effect(() => {
		if (timeline.loading) {
			spinStartedAt = Date.now();
			spinning = true;
			if (spinTimer !== undefined) clearTimeout(spinTimer);
			return;
		}
		// The first settle is the page arriving, not a read anybody asked for: the
		// skeletons already said so, and a spinning Refresh on a feed nobody armed would
		// be the icon lying about a cadence that does not exist.
		if (!settled) {
			settled = true;
			spinning = false;
			return;
		}
		spinTimer = setTimeout(() => {
			spinning = false;
		}, Math.max(0, MIN_SPIN_MS - (Date.now() - spinStartedAt)));
		return () => clearTimeout(spinTimer);
	});

	/**
	 * The busy state rides the icon, not the label. `Refreshing…` is four characters
	 * wider than `Refresh`, and in a header row that widens the button on every tick —
	 * a header that flinches once a second is worse than one that never says it is
	 * working. The word stays, the motion moves, and `aria-busy` carries it to a reader
	 * who cannot see the spin.
	 */
	const refreshIcon = $derived(`h-4 w-4${spinning ? ' animate-spin' : ''}`);

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await timeline.apply(filters);
	}

	async function clearFilters(): Promise<void> {
		filters = {};
		await timeline.clear();
	}

	function handleVisibility(): void {
		if (document.hidden) timeline.stop();
		else timeline.start();
	}

	/**
	 * A deliberate read is the operator taking the cadence back, so the selector
	 * returns to Manual rather than the timer resuming behind the click they were
	 * told was the way to update the page.
	 */
	async function refresh(): Promise<void> {
		timeline.setPoll(null);
		await timeline.refresh();
	}

	function chooseInterval(event: Event & { currentTarget: HTMLSelectElement }): void {
		const chosen = event.currentTarget.value;
		timeline.setPoll(chosen === 'manual' ? null : (Number(chosen) as TimelinePollSeconds));
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
		<div class="flex flex-wrap items-end gap-2">
			{#if applied > 0}
				<button id="timeline-clear" type="button" class="btn btn-sm" onclick={() => void clearFilters()}>
					Clear {applied} filter{applied === 1 ? '' : 's'}
				</button>
			{/if}
			<label class="fieldset">
				<span class="fieldset-legend text-xs">Auto-refresh</span>
				<select
					id="timeline-interval"
					class="select select-sm"
					value={pollChoice}
					onchange={chooseInterval}
				>
					<option value="manual">Manual</option>
					{#each timelinePollIntervals as seconds (seconds)}
						<option value={String(seconds)}>Every {seconds}s</option>
					{/each}
				</select>
			</label>
			<button
				id="timeline-refresh"
				type="button"
				class="btn btn-sm"
				disabled={timeline.busy}
				aria-busy={spinning}
				onclick={() => void refresh()}
			>
				<RefreshCw class={refreshIcon} strokeWidth={2.5} />
				Refresh
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
