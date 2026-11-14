<script lang="ts">
	import { base } from '$app/paths';
	import { untrack } from 'svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import UsageChart from '$lib/components/UsageChart.svelte';
	import {
		USAGE_AXES,
		usageAxisEntryLabel,
		usageAxisTitle
	} from '$lib/components/UsageChart.svelte';
	import { formatShare, formatTimestamp, formatTokenCount } from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
	import {
		USAGE_FILTER_AXES,
		usageFilterValues,
		type UsageAxisChoice,
		type UsageFilterAxis
	} from '$lib/usage-view';
	import {
		USAGE_MAX_DAYS,
		usageCoverageGaps,
		usageOtherWarnings,
		usagePredatesRetained,
		usageRetainedWarning
	} from '$lib/usage-honesty';
	import {
		createUsageStore,
		usageCustomRange,
		inclusiveEnd,
		type UsagePreset,
		type UsageStore
	} from '$lib/stores/usage.svelte';
	import type { UsageDay, UsageTrailEntry } from '$lib/api/types';

	let { store = createUsageStore() }: { store?: UsageStore } = $props();

	$effect(() => {
		// The store's read touches the range state it sends; untracked, so a range
		// change does not tear the mount effect down and start a second read.
		untrack(() => store.start());
		return () => store.stop();
	});

	let from = $state('');
	let to = $state('');
	let customError = $state('');
	// The component stack is the default; the four axes are already in the
	// payload, so switching this never triggers a read. The store owns the
	// choice because the URL carries it.
	const chartAxis = $derived(store.axis);
	const presets: UsagePreset[] = [7, 30, 90];

	/** The human name of one breakdown or filter axis. */
	function axisName(axis: UsageAxisChoice): string {
		if (axis === 'components') return 'Components';
		return USAGE_AXES.find((option) => option.key === axis)?.label ?? axis;
	}

	/** One active filter value, named by the axis it restricts. */
	interface ActiveFilter {
		axis: UsageFilterAxis;
		value: string;
	}

	/**
	 * The chips the operator can undo. They come from the store rather than the
	 * report, so a filter that matched nothing is still a removable chip.
	 */
	const activeFilters = $derived.by((): ActiveFilter[] =>
		USAGE_FILTER_AXES.flatMap((axis) =>
			store.filters[axis].map((value) => ({ axis, value }))
		)
	);

	/**
	 * The values the current view offers for each axis. A value already applied
	 * is a chip, not a second choice, so it is left out of the picker; the
	 * remaining keys are exactly the ones this report contains.
	 */
	const filterOptions = $derived.by(() => {
		const report = store.report;
		return USAGE_FILTER_AXES.map((axis) => ({
			axis,
			label: axisName(axis),
			values: (report ? usageFilterValues(report, axis) : []).filter(
				(value) => !store.filters[axis].includes(value)
			)
		}));
	});

	/** Applies the value chosen in a filter picker, then resets the picker. */
	function chooseFilter(axis: UsageFilterAxis, event: Event): void {
		const select = event.currentTarget as HTMLSelectElement;
		const value = select.value;
		select.value = '';
		if (value) store.addFilter(axis, value);
	}

	// The inputs follow the range the report actually covers, so the control and
	// the table cannot name different dates. Written after the first payload
	// arrives rather than read from the store in the initializer, which would
	// only ever capture the store's first value.
	$effect(() => {
		const report = store.report;
		if (!report) return;
		from = report.since.slice(0, 10);
		to = inclusiveEnd(report.until.slice(0, 10));
	});

	function choose(preset: UsagePreset): void {
		customError = '';
		store.applyPreset(preset);
		from = store.since;
		to = inclusiveEnd(store.until);
	}

	function applyCustom(): void {
		customError = usageCustomRange(from, to)
			? ''
			: 'Choose a start date on or before the end date.';
		if (!customError) store.applyCustom(from, to);
	}

	function dayBadge(day: UsageDay): { status: string; label: string } {
		if (day.executions === 0) return { status: 'noexecutions', label: 'no executions' };
		if (day.unmeasured && day.incomplete) {
			return { status: 'unmeasured', label: 'unmeasured, incomplete' };
		}
		if (day.unmeasured) return { status: 'unmeasured', label: 'unmeasured' };
		if (day.incomplete) return { status: 'incomplete', label: 'incomplete' };
		return { status: 'reported', label: 'reported' };
	}

	const dayColumns: DataColumn<UsageDay>[] = [
		{ key: 'date', label: 'Day', text: (day) => day.date, mono: true },
		{ key: 'executions', label: 'Executions', text: (day) => String(day.executions), align: 'right' },
		{
			key: 'reported',
			label: 'Reported',
			text: (day) => String(day.reportedExecutions),
			align: 'right',
			secondary: true
		},
		{ key: 'trails', label: 'Trails', text: (day) => String(day.distinctTrails), align: 'right' },
		{
			key: 'input',
			label: 'Input',
			text: (day) => formatTokenCount(day.tokens.inputTokens),
			align: 'right'
		},
		{
			key: 'output',
			label: 'Output',
			text: (day) => formatTokenCount(day.tokens.outputTokens),
			align: 'right'
		},
		{
			key: 'reasoning',
			label: 'Reasoning',
			text: (day) => formatTokenCount(day.tokens.reasoningTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'cache-read',
			label: 'Cache read',
			text: (day) => formatTokenCount(day.tokens.cacheReadTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'cache-write',
			label: 'Cache write',
			text: (day) => formatTokenCount(day.tokens.cacheWriteTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'total',
			label: 'Total tokens',
			text: (day) => formatTokenCount(day.tokens.totalTokens),
			align: 'right'
		},
		{
			// Recorded provider cost, never computed from tokens. Blank when no
			// execution on the day recorded one; the count beside it says how many.
			key: 'cost',
			label: 'Recorded cost',
			text: (day) =>
				day.costReportedExecutions > 0
					? `$${day.recordedCostUsd.toFixed(2)}`
					: '—',
			align: 'right'
		},
		{
			key: 'cost-reported',
			label: 'Cost count',
			text: (day) => (day.costReportedExecutions > 0 ? String(day.costReportedExecutions) : '—'),
			align: 'right',
			secondary: true
		},
		{
			key: 'state',
			label: 'Measurement',
			// A day with executions but no reported usage must never read like an
			// empty day: this column names the difference.
			text: (day) => dayBadge(day).label,
			badge: dayBadge
		}
	];

	/**
	 * One period breakdown row, whether the selected stack is the token
	 * components or one of the breakdown axes. A component has no execution
	 * count or recorded cost of its own: a run reports every component at once,
	 * so those cells stay empty rather than claiming an attribution.
	 */
	interface BreakdownRow {
		/** What the row shows: the key, with an adapter's implementation beside it. */
		key: string;
		/** The recorded key a filter takes, when the row can narrow the view. */
		axisKey?: string;
		executions?: number;
		recordedCostUsd?: number;
		costReportedExecutions?: number;
		tokens: number;
	}

	/** The rows of the table, ranked by the selected axis (the server's order). */
	const breakdownRows = $derived.by((): BreakdownRow[] => {
		const report = store.report;
		if (!report) return [];
		if (chartAxis === 'components') {
			const tokens = report.period.tokens;
			return [
				{ key: 'Input', tokens: tokens.inputTokens },
				{ key: 'Output', tokens: tokens.outputTokens },
				{ key: 'Reasoning', tokens: tokens.reasoningTokens },
				{ key: 'Cache read', tokens: tokens.cacheReadTokens },
				{ key: 'Cache write', tokens: tokens.cacheWriteTokens }
			].sort((a, b) => b.tokens - a.tokens);
		}
		return (report.breakdown[chartAxis] ?? []).map((entry) => ({
			key: usageAxisEntryLabel(chartAxis, entry),
			// A model's alias and an adapter's implementation are labels, not
			// grouping decisions: a click filters by the recorded key.
			axisKey: entry.key,
			executions: entry.executions,
			recordedCostUsd: entry.recordedCostUsd,
			costReportedExecutions: entry.costReportedExecutions,
			tokens: entry.tokens.totalTokens
		}));
	});
	const filterAxis = $derived<UsageFilterAxis | null>(
		chartAxis === 'components' ? null : chartAxis
	);

	/**
	 * A breakdown row narrows the view in place: the click adds the row's key
	 * to the filters, and the same click removes it again. The row's name says
	 * which direction it will take, and `pressed` says whether the key is
	 * already applied, so a keyboard or screen-reader operator reads the same
	 * state the chip below shows. Components are not filters, so that stack
	 * leaves its rows as marks.
	 */
	const breakdownRowActivate = $derived(
		filterAxis === null
			? undefined
			: {
					label: (row: BreakdownRow): string => {
						const key = row.axisKey ?? row.key;
						const name = axisName(filterAxis).toLowerCase();
						return store.filters[filterAxis].includes(key)
							? `Clear ${name} filter ${key}`
							: `Add ${name} filter ${key}`;
					},
					pressed: (row: BreakdownRow): boolean =>
						store.filters[filterAxis].includes(row.axisKey ?? row.key),
					onselect: (row: BreakdownRow): void =>
						store.toggleFilter(filterAxis, row.axisKey ?? row.key)
				}
	);
	const axisTotalTokens = $derived(store.report?.period.tokens.totalTokens ?? 0);
	const breakdownTitle = $derived(
		chartAxis === 'components'
			? 'Period breakdown by component'
			: `Period breakdown by ${usageAxisTitle(chartAxis).toLowerCase()}`
	);
	const breakdownColumns = $derived.by((): DataColumn<BreakdownRow>[] => [
		{
			key: 'key',
			label: 'Key',
			// An adapter's implementation or a model's alias stays beside the key;
			// it is a label, never the grouping decision.
			text: (row) => row.key,
			mono: true
		},
		{
			key: 'executions',
			label: 'Executions',
			text: (row) => (row.executions === undefined ? '—' : String(row.executions)),
			align: 'right'
		},
		{
			key: 'tokens',
			label: 'Tokens',
			text: (row) => formatTokenCount(row.tokens),
			align: 'right'
		},
		{
			key: 'share',
			label: 'Share',
			text: (row) => formatShare(row.tokens, axisTotalTokens),
			align: 'right'
		},
		{
			key: 'cost',
			// Recorded provider cost, only where an execution reported it, with
			// the count of executions that did. Never derived from tokens.
			label: 'Recorded cost',
			text: (row) =>
				row.costReportedExecutions && row.recordedCostUsd !== undefined
					? `$${row.recordedCostUsd.toFixed(2)} (${row.costReportedExecutions})`
					: '—',
			align: 'right'
		}
	]);

	/**
	 * A row's identity on screen: its recorded title, or the trail id when no
	 * title exists. The id stays searchable either way, because that is what the
	 * operator copied from a run or an event.
	 */
	function trailLabel(trail: UsageTrailEntry): string {
		return trail.title?.trim() || trail.traceId;
	}

	const trailColumns: DataColumn<UsageTrailEntry>[] = [
		{
			key: 'trail',
			label: 'Trail',
			text: trailLabel,
			searchText: (trail) => [trail.traceId, trail.title ?? ''].join(' '),
			// The one navigation this page performs: the trail detail already
			// owns that trail's usage, so the link goes there instead of
			// re-implementing a second per-trail report here.
			href: (trail) => traceDetailPath(base, trail.traceId),
			grow: true
		},
		{
			key: 'executions',
			label: 'Executions',
			text: (trail) => String(trail.executions),
			align: 'right'
		},
		{
			key: 'reported',
			label: 'Reported',
			text: (trail) => String(trail.reportedExecutions),
			align: 'right',
			secondary: true
		},
		{
			key: 'tokens',
			label: 'Total tokens',
			text: (trail) => formatTokenCount(trail.tokens.totalTokens),
			align: 'right'
		},
		{
			// The provider's own number, shown only where an execution reported
			// it, with the count that did. Never derived from tokens.
			key: 'cost',
			label: 'Recorded cost',
			text: (trail) =>
				trail.costReportedExecutions > 0
					? `$${trail.recordedCostUsd.toFixed(2)} (${trail.costReportedExecutions})`
					: '—',
			align: 'right'
		}
	];

	const trailRanking = $derived(store.report?.trails);
	const shownTrailTokens = $derived(
		trailRanking
			? trailRanking.entries.reduce((sum, trail) => sum + trail.tokens.totalTokens, 0)
			: 0
	);
	/**
	 * The sentence that makes the table honest about its own depth: the rows on
	 * screen plus the trails the table leaves out are the period total, so an
	 * operator can see that the top ten are not the whole colony.
	 */
	const trailReconciliation = $derived.by((): string => {
		if (!trailRanking || !store.report) return '';
		const periodTokens = store.report.period.tokens.totalTokens;
		if (trailRanking.outsideTrails > 0) {
			const word =
				trailRanking.outsideTrails === 1 ? 'measured trail' : 'measured trails';
			return (
				`The rows above account for ${formatTokenCount(shownTrailTokens)} tokens, and the ` +
				`${trailRanking.outsideTrails} ${word} outside this table account for ` +
				`${formatTokenCount(trailRanking.outsideTokens)}, the period's ` +
				`${formatTokenCount(periodTokens)} tokens in total.`
			);
		}
		return (
			`Every trail that reported usage is listed, and the rows account for all ` +
			`${formatTokenCount(periodTokens)} tokens reported in this period.`
		);
	});

	const period = $derived(store.report?.period);
	const report = $derived(store.report);
	/**
	 * The five degraded states the page must not collapse into one another: a
	 * range with no executions, a range whose executions reported nothing, a
	 * range older than the retained record, a range longer than the cap, and a
	 * read that failed. Each has its own copy and its own `data-usage-state`.
	 */
	const noExecutions = $derived(
		period !== undefined && period.executions === 0 && !store.hasFilters
	);
	const onlyUnmeasured = $derived(
		period !== undefined && period.executions > 0 && period.reportedExecutions === 0
	);
	const predatesRetained = $derived(report ? usagePredatesRetained(report) : false);
	const retainedWarning = $derived(report ? usageRetainedWarning(report) : undefined);
	const otherWarnings = $derived(report ? usageOtherWarnings(report) : []);
	const gaps = $derived(report ? usageCoverageGaps(report) : { undated: 0, unreadable: 0 });
	const readFailed = $derived(store.lastErrorKind === 'read-failed');
	const rangeTooLong = $derived(store.lastErrorKind === 'range-too-long');
	const components = $derived(
		period
			? [
					{ label: 'Input', value: period.tokens.inputTokens },
					{ label: 'Output', value: period.tokens.outputTokens },
					{ label: 'Reasoning', value: period.tokens.reasoningTokens },
					{ label: 'Cache read', value: period.tokens.cacheReadTokens },
					{ label: 'Cache write', value: period.tokens.cacheWriteTokens }
				]
			: []
	);
	const coverage = $derived(
		period ? `${period.reportedExecutions} of ${period.executions} reported` : '—'
	);
	/** The coverage tile's full text, including the edges the reader excluded. */
	const coverageHints = $derived.by((): string[] => {
		if (!report || !period) return ['No report has been read yet'];
		const hints = [`${period.reportedExecutions} of ${period.executions} executions reported usage`];
		if (gaps.undated > 0) {
			hints.push(
				`${gaps.undated} execution${gaps.undated === 1 ? '' : 's'} excluded with no usable start time`
			);
		}
		if (gaps.unreadable > 0) {
			hints.push(
				`${gaps.unreadable} run record${gaps.unreadable === 1 ? '' : 's'} unreadable, so coverage is reduced`
			);
		}
		hints.push(
			report.earliestRetainedAt
				? `Retained record begins ${formatTimestamp(report.earliestRetainedAt)}`
				: 'No retained execution record is dated'
		);
		return hints;
	});
	/**
	 * The coverage area's visible sentence. A gap named only in a tooltip is a
	 * gap an operator can miss, so the excluded-undated and unreadable counts are
	 * printed beside the reported-of-executions number rather than hidden behind
	 * a hover.
	 */
	const coverageSentence = $derived.by((): string => {
		if (!report || !period) return '';
		const clauses = [`${period.reportedExecutions} of ${period.executions} executions reported usage`];
		if (gaps.undated > 0) {
			clauses.push(
				`${gaps.undated} execution${gaps.undated === 1 ? '' : 's'} excluded from the daily buckets because they have no usable start time`
			);
		}
		if (gaps.unreadable > 0) {
			clauses.push(
				`${gaps.unreadable} run record${gaps.unreadable === 1 ? '' : 's'} could not be read, so coverage is reduced`
			);
		}
		return `${clauses.join('; ')}.`;
	});
	const rangeLabel = $derived(
		report
			? `${report.since} to ${report.until} (${report.timezone})`
			: `${store.since} to ${store.until} (${store.timezone})`
	);
</script>

<svelte:head>
	<title>Usage · Queen Console</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-2">
		<h1 class="text-3xl font-bold">Usage</h1>
		<p class="text-base-content/70">
			Token volume the colony recorded over retained runs, split into input, output, reasoning,
			cache read, and cache write. These are tokens, not money: a cache-heavy model can show
			millions of tokens at a small cost. Recorded provider cost appears in the daily table only
			where an adapter reported it, with the number of executions that did.
		</p>
	</header>

	<div class="flex flex-wrap items-end gap-3">
		<div class="join" role="group" aria-label="Usage range">
			{#each presets as days (days)}
				<button
					class="btn join-item btn-sm"
					class:btn-active={store.preset === days}
					aria-pressed={store.preset === days}
					onclick={() => choose(days)}
				>
					{days} days
				</button>
			{/each}
			<button
				class="btn join-item btn-sm"
				class:btn-active={store.preset === 'custom'}
				aria-pressed={store.preset === 'custom'}
				onclick={applyCustom}
			>
				Custom
			</button>
		</div>

		<div class="flex flex-wrap items-end gap-2">
			<label class="form-control">
				<span class="label-text text-xs">From</span>
				<input class="input input-sm" type="date" aria-label="From date" bind:value={from} />
			</label>
			<label class="form-control">
				<span class="label-text text-xs">To</span>
				<input class="input input-sm" type="date" aria-label="To date" bind:value={to} />
			</label>
			<button class="btn btn-sm" onclick={applyCustom}>Apply</button>
		</div>

		<button
			class="btn btn-sm btn-outline ml-auto"
			onclick={() => store.refresh()}
			disabled={store.loading}
			title="Scan the retained runs again, bypassing the short-lived cache"
		>
			Refresh
		</button>
	</div>

	{#if customError}
		<div class="alert alert-warning" role="alert"><span>{customError}</span></div>
	{/if}

	{#if rangeTooLong}
		<section
			class="alert alert-warning"
			role="alert"
			data-usage-state="overlong"
			aria-labelledby="usage-overlong-title"
		>
			<div class="space-y-1">
				<h2 id="usage-overlong-title" class="text-lg font-semibold">
					This range is longer than the report's cap
				</h2>
				<p class="text-sm">
					The report covers at most {USAGE_MAX_DAYS} days at a time, so this range was not read.
					{store.report
						? 'The last successful report is still shown below.'
						: 'Choose a shorter range to read one.'}
				</p>
			</div>
		</section>
	{:else if readFailed && !store.report}
		<section
			class="alert alert-error"
			role="alert"
			data-usage-state="error"
			aria-labelledby="usage-read-failed-title"
		>
			<div class="space-y-1">
				<h2 id="usage-read-failed-title" class="text-lg font-semibold">
					The report could not be read
				</h2>
				<p class="text-sm">{store.lastError}</p>
			</div>
		</section>
	{/if}

	<section class="space-y-3" aria-labelledby="usage-filters">
		<div class="flex flex-wrap items-center justify-between gap-2">
			<h2 id="usage-filters" class="text-sm font-medium">Filters</h2>
			{#if activeFilters.length > 0}
				<button class="btn btn-ghost btn-xs" onclick={() => store.clearFilters()}>
					Clear all filters
				</button>
			{/if}
		</div>

		<div class="flex flex-wrap items-end gap-3">
			{#if store.report}
				{#each filterOptions as option (option.axis)}
					<label class="form-control">
						<span class="label-text text-xs">{option.label}</span>
						<select
							class="select select-sm"
							aria-label={`Add ${option.label} filter`}
							value=""
							onchange={(event) => chooseFilter(option.axis, event)}
						>
							<option value="">
								{option.values.length > 0
									? `Add ${option.label.toLowerCase()}`
									: 'No values in this view'}
							</option>
							{#each option.values as value (value)}
								<option value={value}>{value}</option>
							{/each}
						</select>
					</label>
				{/each}
			{/if}
		</div>

		{#if store.day || activeFilters.length > 0}
			<ul class="flex flex-wrap gap-2" aria-label="Active filters">
				{#if store.day}
					<li class="badge badge-lg gap-1">
						<span>
							Day: <span class="font-mono">{store.day}</span>
						</span>
						<button
							type="button"
							class="btn btn-ghost btn-xs"
							aria-label={`Remove day filter ${store.day}`}
							onclick={() => store.clearDay()}
						>
							×
						</button>
					</li>
				{/if}
				{#each activeFilters as chip (`${chip.axis}:${chip.value}`)}
					<li class="badge badge-lg gap-1">
						<span>
							{axisName(chip.axis)}: <span class="font-mono">{chip.value}</span>
						</span>
						<button
							type="button"
							class="btn btn-ghost btn-xs"
							aria-label={`Remove ${chip.axis} filter ${chip.value}`}
							onclick={() => store.removeFilter(chip.axis, chip.value)}
						>
							×
						</button>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="text-xs text-base-content/60">
				No filters. Add one above to isolate an adapter, model, Bee, or purpose; the range and
				the day boundaries stay as they are.
			</p>
		{/if}
	</section>

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-40 w-full"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if store.report && period}
		<p class="text-xs text-base-content/60" data-usage-observed-at={store.report.observedAt}>
			{rangeLabel} · observed {formatTimestamp(store.report.observedAt)} · start-time buckets, so a
			run crossing midnight stays on its start day
		</p>

		{#if readFailed}
			<!-- The last good report is still the numbers on screen, so the error
			     sits beside it rather than replacing it. -->
			<section
				class="alert alert-error"
				role="alert"
				data-usage-state="error"
				aria-labelledby="usage-stale-title"
			>
				<div class="space-y-1">
					<h2 id="usage-stale-title" class="text-lg font-semibold">
						This read failed; the report below is the last good one
					</h2>
					<p class="text-sm">{store.lastError}</p>
					<p class="text-xs">
						Observed {formatTimestamp(store.report.observedAt)}. Nothing below changed; press
						Refresh to try again.
					</p>
				</div>
			</section>
		{/if}

		{#if otherWarnings.length > 0}
			<div class="alert alert-warning" role="status" data-usage-warnings>
				<ul class="list-disc space-y-1 pl-5">
					{#each otherWarnings as warning, index (index)}
						<li>{warning.detail}</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if predatesRetained}
			<section
				class="alert alert-warning"
				role="status"
				data-usage-state="predates"
				aria-labelledby="usage-predates-title"
			>
				<div class="space-y-1">
					<h2 id="usage-predates-title" class="text-lg font-semibold">
						This range begins before the retained record
					</h2>
					<p class="text-sm">
						{retainedWarning?.detail}. The gap is unmeasured, not a quiet period: nothing retained
						says the colony had no executions then.
					</p>
				</div>
			</section>
		{/if}

		{#if noExecutions}
			<section
				class="alert"
				role="status"
				data-usage-state="none"
				aria-labelledby="usage-none-title"
			>
				<div class="space-y-1">
					<h2 id="usage-none-title" class="text-lg font-semibold">No executions in this range</h2>
					<p class="text-sm">
						No retained run started on these days. The range is a genuine zero, not a failed read
						and not an unmeasured one; the daily table lists every day zero-filled.
					</p>
				</div>
			</section>
		{:else if onlyUnmeasured}
			<section
				class="alert alert-warning"
				role="status"
				data-usage-state="unmeasured"
				aria-labelledby="usage-unmeasured-title"
			>
				<div class="space-y-1">
					<h2 id="usage-unmeasured-title" class="text-lg font-semibold">
						Executions in this range reported no usage
					</h2>
					<p class="text-sm">
						{period.executions} execution{period.executions === 1 ? '' : 's'} across
						{period.distinctTrails} trail{period.distinctTrails === 1 ? '' : 's'} finished without a
						usage record. The zero below is the absence of a measurement, not an observed zero.
					</p>
				</div>
			</section>
		{/if}

		{#if store.hasFilters && period.executions === 0}
			{@const filtersLabel = activeFilters
				.map((chip) => `${chip.axis} ${chip.value}`)
				.join(', ')}
			<section
				class="alert"
				role="status"
				aria-labelledby="usage-empty-title"
				data-usage-empty="filters"
			>
				<div class="space-y-1">
					<h2 id="usage-empty-title" class="text-lg font-semibold">
						No executions match the active filters
					</h2>
					<p class="text-sm">
						Nothing retained in {store.report.since} to {store.report.until} matches
						{filtersLabel}. The range and its {store.report.days.length} zero-filled days are
						unchanged, so removing a filter restores what this range holds.
					</p>
				</div>
				<button class="btn btn-sm" onclick={() => store.clearFilters()}>
					Clear filters to see the range
				</button>
			</section>
		{:else}
			<section class="space-y-3" aria-labelledby="usage-headline">
				<h2 id="usage-headline" class="text-xl font-semibold">Token volume</h2>

				<div class="rounded-box border border-base-300 bg-base-100 p-4">
					<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
						{#each components as component (component.label)}
							<div>
								<p class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">
									{component.label}
								</p>
								<p class="text-2xl leading-tight font-bold">{formatTokenCount(component.value)}</p>
							</div>
						{/each}
						<div>
							<p class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Total</p>
							<p class="text-2xl leading-tight font-bold">{formatTokenCount(period.tokens.totalTokens)}</p>
						</div>
					</div>
					<p class="mt-3 text-xs text-base-content/60">
						Total is the sum of these reported components. Reasoning is not folded into output, and
						cache kinds are not folded into input.
					</p>
				</div>

				<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
					<StatTile
						label="Executions"
						value={String(period.executions)}
						hint={[`${period.reportedExecutions} reported usage`, `${period.executions - period.reportedExecutions} did not`]}
					/>
					<StatTile label="Trails" value={String(period.distinctTrails)} />
					<StatTile label="Coverage" value={coverage} hint={coverageHints} />
				</div>
				<div class="space-y-1 text-xs text-base-content/60" data-usage-coverage>
					<p>{coverageSentence}</p>
					{#if store.report.earliestRetainedAt}
						<p>
							The report covers retained trails only: the retained record begins
							{formatTimestamp(store.report.earliestRetainedAt)}. Spend from trails whose records
							were pruned is absent rather than counted as zero.
						</p>
					{:else}
						<p>
							The report covers retained trails only, and no retained execution record is dated.
							Spend from trails whose records were pruned is absent rather than counted as zero.
						</p>
					{/if}
					{#if predatesRetained}
						<p data-usage-gap="predates">
							This range starts before that record, so its earlier days are unmeasured; they are
							not days the colony was quiet.
						</p>
					{/if}
				</div>
			</section>

			<div class="flex flex-wrap items-center gap-3">
				<span id="usage-axis-label" class="text-sm font-medium">Breakdown</span>
				<div class="join" role="group" aria-labelledby="usage-axis-label">
					<button
						class="btn join-item btn-sm"
						class:btn-active={chartAxis === 'components'}
						aria-pressed={chartAxis === 'components'}
						onclick={() => store.setAxis('components')}
					>
						Components
					</button>
					{#each USAGE_AXES as option (option.key)}
						<button
							class="btn join-item btn-sm"
							class:btn-active={chartAxis === option.key}
							aria-pressed={chartAxis === option.key}
							onclick={() => store.setAxis(option.key)}
						>
							{option.label}
						</button>
					{/each}
				</div>
			</div>

			<UsageChart
				report={store.report}
				axis={chartAxis}
				onSelectDay={(date) => store.selectDay(date)}
				selectedDay={store.day}
			/>

			<section class="space-y-3" aria-labelledby="usage-breakdown">
				<h2 id="usage-breakdown" class="text-xl font-semibold">{breakdownTitle}</h2>
				<DataTable
					label="Breakdown keys"
					columns={breakdownColumns}
					rows={breakdownRows}
					rowKey={(row) => row.key}
					emptyMessage="No executions reported usage in this range."
					filterLabel="Filter keys"
					filterPlaceholder="key"
					pageSize={0}
					stateKey="breakdown"
					rowActivate={breakdownRowActivate}
				/>
				<p class="text-xs text-base-content/50">
					Ranked by token volume. Share is the key's part of the {formatTokenCount(
						axisTotalTokens
					)} tokens reported in this period; the chart draws the top keys and groups the rest as
					other, while this table lists every key. A component has no execution count or cost of its
					own, because a run reports every component at once. Recorded cost appears only where an
					execution reported it, with the count that did.
					{#if filterAxis}
						Selecting a row adds its key to the filters without leaving the range, and selecting it
						again removes it.
					{/if}
				</p>
			</section>

			<section class="space-y-3" aria-labelledby="usage-trails">
				<h2 id="usage-trails" class="text-xl font-semibold">Top trails</h2>
				{#if trailRanking && trailRanking.entries.length > 0}
					<DataTable
						label="Top trails table"
						columns={trailColumns}
						rows={trailRanking.entries}
						rowKey={(trail) => trail.traceId}
						emptyMessage="No trail reported usage in this range."
						filterLabel="Filter trails"
						filterPlaceholder="trail or title"
						pageSize={0}
						stateKey="trails"
					/>
					<p class="text-xs text-base-content/50">
						Ranked by reported tokens. The trail link opens its existing detail, where that trail's
						usage already lives. A trail whose runs reported nothing is not listed as a zero-token
						trail: that is a coverage gap, not a free one. {trailReconciliation}
					</p>
				{:else}
					<p class="text-base-content/70">
						No trail reported usage in this range. A trail whose runs reported nothing is a coverage
						gap, not a zero-token trail, so it is not listed.
					</p>
				{/if}
			</section>

			<section class="space-y-3" aria-labelledby="usage-daily">
				<h2 id="usage-daily" class="text-xl font-semibold">Daily volume</h2>
				{#if period.executions === 0}
					<p class="text-base-content/70">
						Every day below is zero-filled, so two ranges share the same axis. A day with
						executions but no reported usage reads <strong>unmeasured</strong> instead.
					</p>
				{/if}
				<DataTable
					label="Daily usage"
					columns={dayColumns}
					rows={store.report.days}
					rowKey={(day) => day.date}
					emptyMessage="No days in this range."
					filterLabel="Filter days"
					filterPlaceholder="day or measurement"
					pageSize={0}
					stateKey="daily"
				/>
				<p class="text-xs text-base-content/50">
					Recorded cost is the provider's own number, shown only where an adapter reported it, with
					the count of executions that did. It is never estimated from tokens.
				</p>
			</section>
		{/if}
	{/if}
</div>
