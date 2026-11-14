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
		usageAxisTitle,
		type UsageChartAxis
	} from '$lib/components/UsageChart.svelte';
	import { formatShare, formatTimestamp, formatTokenCount } from '$lib/format';
	import { traceDetailPath } from '$lib/navigation';
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
	// payload, so switching this never triggers a read.
	let chartAxis = $state<UsageChartAxis>('components');
	const presets: UsagePreset[] = [7, 30, 90];

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
		key: string;
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
			executions: entry.executions,
			recordedCostUsd: entry.recordedCostUsd,
			costReportedExecutions: entry.costReportedExecutions,
			tokens: entry.tokens.totalTokens
		}));
	});
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
	const rangeLabel = $derived(
		store.report
			? `${store.report.since} to ${store.report.until} (${store.report.timezone})`
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

		<button class="btn btn-sm btn-outline ml-auto" onclick={() => store.refresh()} disabled={store.loading}>
			Refresh
		</button>
	</div>

	{#if customError}
		<div class="alert alert-warning" role="alert"><span>{customError}</span></div>
	{/if}

	{#if store.lastError}
		<div class="alert alert-error" role="alert">
			<span>{store.lastError}</span>
		</div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-40 w-full"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if store.report && period}
		<p class="text-xs text-base-content/60">
			{rangeLabel} · observed {formatTimestamp(store.report.observedAt)} · start-time buckets, so a
			run crossing midnight stays on its start day
		</p>

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
				<StatTile
					label="Coverage"
					value={coverage}
					hint={[
						store.report.earliestRetainedAt
							? `Retained record begins ${formatTimestamp(store.report.earliestRetainedAt)}`
							: 'No retained execution record is dated'
					]}
				/>
			</div>
		</section>

		{#if store.report.warnings.length > 0}
			<div class="alert alert-warning" role="status">
				<ul class="list-disc space-y-1 pl-5">
					{#each store.report.warnings as warning, index (index)}
						<li>{warning.detail}</li>
					{/each}
				</ul>
			</div>
		{/if}

		<div class="flex flex-wrap items-center gap-3">
			<span id="usage-axis-label" class="text-sm font-medium">Breakdown</span>
			<div class="join" role="group" aria-labelledby="usage-axis-label">
				<button
					class="btn join-item btn-sm"
					class:btn-active={chartAxis === 'components'}
					aria-pressed={chartAxis === 'components'}
					onclick={() => (chartAxis = 'components')}
				>
					Components
				</button>
				{#each USAGE_AXES as option (option.key)}
					<button
						class="btn join-item btn-sm"
						class:btn-active={chartAxis === option.key}
						aria-pressed={chartAxis === option.key}
						onclick={() => (chartAxis = option.key)}
					>
						{option.label}
					</button>
				{/each}
			</div>
		</div>

		<UsageChart report={store.report} axis={chartAxis} />

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
			/>
			<p class="text-xs text-base-content/50">
				Ranked by token volume. Share is the key's part of the {formatTokenCount(
					axisTotalTokens
				)} tokens reported in this period; the chart draws the top keys and groups the rest as
				other, while this table lists every key. A component has no execution count or cost of its
				own, because a run reports every component at once. Recorded cost appears only where an
				execution reported it, with the count that did.
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
					No executions in this range. The days below are still listed, zero-filled, so two ranges
					share the same axis.
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
</div>
