<script module lang="ts">
	import { formatTokenCount } from '$lib/format';
	import type { UsageAxisEntry, UsageDay, UsageReport } from '$lib/api/types';

	/**
	 * The series the daily chart stacks by default, in stack order. The order is
	 * part of the contract: the palette is fixed and both the chart and its
	 * legend read from this one list, so a component cannot be coloured in the
	 * chart and named differently in the legend.
	 */
	export const USAGE_COMPONENTS = [
		{ key: 'input', label: 'Input' },
		{ key: 'output', label: 'Output' },
		{ key: 'reasoning', label: 'Reasoning' },
		{ key: 'cache-read', label: 'Cache read' },
		{ key: 'cache-write', label: 'Cache write' }
	] as const;

	export type UsageSeriesKey = (typeof USAGE_COMPONENTS)[number]['key'];

	/** The breakdown axes the chart can stack instead of the components. */
	export const USAGE_AXES = [
		{ key: 'adapter', label: 'Adapter' },
		{ key: 'model', label: 'Model' },
		{ key: 'bee', label: 'Bee' },
		{ key: 'purpose', label: 'Purpose' }
	] as const;

	export type UsageAxisName = (typeof USAGE_AXES)[number]['key'];

	/**
	 * What the chart stacks: the token components, or one breakdown axis. The
	 * report already carries all four axes, so switching this costs no read.
	 */
	export type UsageChartAxis = 'components' | UsageAxisName;

	/** The series id the chart gives the keys a palette cannot fit. */
	export const USAGE_OTHER_SERIES = 'other';

	/**
	 * The fixed ordered palette. Every entry is a DaisyUI theme token, so a
	 * `data-theme` switch repaints the chart without a reload and without a
	 * resolved colour anywhere in the component: the mark carries
	 * `var(--color-*)` and the legend carries the matching utility class.
	 *
	 * `info` is deliberately absent: it is the same value as `primary` in all
	 * four Catppuccin themes, so a series mapped to it would render as a
	 * duplicate of the first series on four of the six shipped themes.
	 */
	export const USAGE_PALETTE = [
		{ token: '--color-primary', swatch: 'bg-primary' },
		{ token: '--color-secondary', swatch: 'bg-secondary' },
		{ token: '--color-accent', swatch: 'bg-accent' },
		{ token: '--color-warning', swatch: 'bg-warning' },
		{ token: '--color-success', swatch: 'bg-success' }
	] as const;

	/**
	 * The palette slot for a series key. The five token components have a fixed
	 * order; any other key (a breakdown axis key) hashes to a slot, so the same
	 * key keeps its colour across reloads, ranges, and axes.
	 */
	export function usagePaletteIndex(key: string): number {
		const known = USAGE_COMPONENTS.findIndex((component) => component.key === key);
		if (known >= 0) return known % USAGE_PALETTE.length;
		// FNV-1a: small, stable, and good enough to spread model names over slots.
		let hash = 0x811c9dc5;
		for (let index = 0; index < key.length; index += 1) {
			hash ^= key.charCodeAt(index);
			hash = Math.imul(hash, 0x01000193);
		}
		return (hash >>> 0) % USAGE_PALETTE.length;
	}

	/** A theme-token colour reference for a series key, e.g. `var(--color-primary)`. */
	export function usageSeriesColor(key: string): string {
		return `var(${USAGE_PALETTE[usagePaletteIndex(key)].token})`;
	}

	/** The Tailwind swatch class for a series key, for an HTML legend. */
	export function usageSeriesSwatch(key: string): string {
		return USAGE_PALETTE[usagePaletteIndex(key)].swatch;
	}

	/** One stack series: its identity, its label, and its resolved theme slot. */
	export interface UsageSeries {
		id: string;
		label: string;
		/** A `--color-*` token name, never a colour value. */
		token: string;
		swatch: string;
	}

	/**
	 * The identity of one axis entry. An adapter keeps its implementation beside
	 * its name, because a proxy and its v2 implementation are two groups even
	 * though the adapter name is one; every other axis is its key alone.
	 */
	export function usageAxisEntryId(axis: UsageChartAxis, entry: UsageAxisEntry): string {
		if (axis === 'adapter' && entry.label) return `${entry.key} · ${entry.label}`;
		return entry.key;
	}

	/** What a reader sees for one axis key, with its secondary label beside it. */
	export function usageAxisEntryLabel(axis: UsageChartAxis, entry: UsageAxisEntry): string {
		if (!entry.label) return entry.key;
		return `${entry.key} · ${entry.label}`;
	}

	/**
	 * The series the chart draws for the selected axis.
	 *
	 * The component stack is fixed. A breakdown axis is ranked by the report, and
	 * the number of series is bounded by the palette: the top keys keep a slot and
	 * the remainder become one `other` series. The table below still lists every
	 * key, so nothing is hidden — only the marks are bounded.
	 */
	export function usageChartSeries(
		report: UsageReport,
		axis: UsageChartAxis = 'components'
	): UsageSeries[] {
		const named: { id: string; label: string }[] =
			axis === 'components'
				? USAGE_COMPONENTS.map((component) => ({ id: component.key, label: component.label }))
				: (() => {
						const entries = report.breakdown[axis] ?? [];
						const top = entries.slice(0, USAGE_PALETTE.length - 1);
						const rest = entries.slice(top.length);
						const series = top.map((entry) => ({
							id: usageAxisEntryId(axis, entry),
							label: usageAxisEntryLabel(axis, entry)
						}));
						if (rest.length > 0) {
							series.push({ id: USAGE_OTHER_SERIES, label: `Other (${rest.length})` });
						}
						return series;
					})();

		// Resolve slots with linear probing, so two keys that hash to the same
		// slot stay two colours. A key's slot is still derived from the key, and
		// the set of series is the report's own ranking, so the same view keeps
		// the same colours.
		const used = new Set<number>();
		return named.map(({ id, label }) => {
			let index = usagePaletteIndex(id);
			for (let probe = 0; probe < USAGE_PALETTE.length && used.has(index); probe += 1) {
				index = (index + 1) % USAGE_PALETTE.length;
			}
			used.add(index);
			return { id, label, ...USAGE_PALETTE[index] };
		});
	}

	/** One stack segment: a day, a series, and its reported volume. */
	export interface UsageChartRow {
		date: string;
		series: string;
		label: string;
		tokens: number;
		/** The day's reported total, repeated so a tooltip can name it. */
		dayTotal: number;
		unmeasured: boolean;
		incomplete: boolean;
	}

	/**
	 * Long-form rows for the stacked chart, one per series per day. Everything
	 * comes from the report the page already holds; the chart never reads usage
	 * itself. A row is emitted even for a zero series, so the stack's sum is the
	 * day's total exactly rather than the sum of its non-zero parts.
	 */
	export function usageChartRows(
		report: UsageReport,
		axis: UsageChartAxis = 'components',
		series: UsageSeries[] = usageChartSeries(report, axis)
	): UsageChartRow[] {
		if (axis === 'components') {
			return report.days.flatMap((day) =>
				USAGE_COMPONENTS.map((component, index) => ({
					date: day.date,
					series: component.key,
					label: component.label,
					tokens: [
						day.tokens.inputTokens,
						day.tokens.outputTokens,
						day.tokens.reasoningTokens,
						day.tokens.cacheReadTokens,
						day.tokens.cacheWriteTokens
					][index],
					dayTotal: day.tokens.totalTokens,
					unmeasured: day.unmeasured,
					incomplete: day.incomplete
				}))
			);
		}

		const seriesIds = new Set(series.map((entry) => entry.id));
		return report.days.flatMap((day) => {
			const totals = new Map<string, number>();
			for (const entry of day.breakdown[axis] ?? []) {
				const id = usageAxisEntryId(axis, entry);
				const target = seriesIds.has(id) ? id : USAGE_OTHER_SERIES;
				totals.set(target, (totals.get(target) ?? 0) + entry.tokens.totalTokens);
			}
			return series.map((entry) => ({
				date: day.date,
				series: entry.id,
				label: entry.label,
				tokens: totals.get(entry.id) ?? 0,
				dayTotal: day.tokens.totalTokens,
				unmeasured: day.unmeasured,
				incomplete: day.incomplete
			}));
		});
	}

	/** A day is flagged when any of its executions is unmeasured or unfinished. */
	export function usageFlaggedDays(days: UsageDay[]): UsageDay[] {
		return days.filter((day) => day.unmeasured || day.incomplete);
	}

	/** The state word a flagged day carries, or `null` for an ordinary day. */
	export function usageDayState(day: UsageDay): 'unmeasured' | 'incomplete' | null {
		if (day.unmeasured) return 'unmeasured';
		if (day.incomplete) return 'incomplete';
		return null;
	}

	/** The axis label a reader sees, or the default stack's own name. */
	export function usageAxisTitle(axis: UsageChartAxis): string {
		if (axis === 'components') return 'components';
		return USAGE_AXES.find((option) => option.key === axis)?.label ?? axis;
	}

	/** The summary an assistive technology reads instead of the marks. */
	export function usageChartSummary(
		report: UsageReport,
		axis: UsageChartAxis = 'components'
	): string {
		const first = report.days.at(0)?.date ?? report.since;
		const last = report.days.at(-1)?.date ?? report.until;
		const period = report.period;
		const unmeasured = report.days.filter((day) => day.unmeasured).length;
		const incomplete = report.days.filter((day) => day.incomplete).length;
		const states = [
			unmeasured > 0 ? `${unmeasured} ${unmeasured === 1 ? 'day is' : 'days are'} unmeasured` : '',
			incomplete > 0 ? `${incomplete} ${incomplete === 1 ? 'day is' : 'days are'} incomplete` : ''
		]
			.filter(Boolean)
			.join(' and ');

		if (axis !== 'components') {
			const entries = report.breakdown[axis] ?? [];
			const top = entries
				.slice(0, USAGE_PALETTE.length)
				.map((entry) => {
					const share =
						period.tokens.totalTokens > 0
							? ` (${((entry.tokens.totalTokens / period.tokens.totalTokens) * 100).toFixed(1)}%)`
							: '';
					return `${usageAxisEntryLabel(axis, entry)} ${formatTokenCount(entry.tokens.totalTokens)}${share}`;
				})
				.join(', ');
			return [
				`Daily token volume by ${usageAxisTitle(axis).toLowerCase()} from ${first} to ${last} (${report.timezone}):`,
				`${formatTokenCount(period.tokens.totalTokens)} tokens in total across ${entries.length} ${entries.length === 1 ? 'key' : 'keys'}.`,
				top ? `Top keys: ${top}.` : '',
				states ? `${states}.` : '',
				'The breakdown table below lists every key and the daily table holds the same numbers.'
			]
				.filter(Boolean)
				.join(' ');
		}

		const totals = USAGE_COMPONENTS.map(
			(component, index) =>
				`${component.label.toLowerCase()} ${formatTokenCount(
					[
						period.tokens.inputTokens,
						period.tokens.outputTokens,
						period.tokens.reasoningTokens,
						period.tokens.cacheReadTokens,
						period.tokens.cacheWriteTokens
					][index]
				)}`
		).join(', ');
		return [
			`Daily token volume from ${first} to ${last} (${report.timezone}):`,
			`${formatTokenCount(period.tokens.totalTokens)} tokens in total (${totals})`,
			`across ${period.executions} executions, ${period.reportedExecutions} of which reported usage.`,
			states ? `${states}.` : '',
			'The daily table below holds the same numbers.'
		]
			.filter(Boolean)
			.join(' ');
	}
</script>

<script lang="ts">
	import { Axis, Bars, Chart, Svg, Tooltip } from 'layerchart';

	let {
		report,
		axis = 'components',
		onSelectDay,
		selectedDay = null
	}: {
		report: UsageReport;
		axis?: UsageChartAxis;
		/**
		 * Narrows the view to one local day. Absent for a chart that is only read;
		 * the column stays a mark rather than becoming a control.
		 */
		onSelectDay?: (date: string) => void;
		/** The day the view is already narrowed to, so the control reports it. */
		selectedDay?: string | null;
	} = $props();

	const chartId = $props.id();
	const series = $derived(usageChartSeries(report, axis));
	const rows = $derived(usageChartRows(report, axis, series));
	const seriesKeys = $derived(series.map((entry) => entry.id));
	const seriesColors = $derived(series.map((entry) => `var(${entry.token})`));
	const flagged = $derived(usageFlaggedDays(report.days));
	const summary = $derived(usageChartSummary(report, axis));
	const rowsByDay = $derived.by(() => {
		const grouped = new Map<string, UsageChartRow[]>();
		for (const row of rows) {
			const day = grouped.get(row.date);
			if (day) day.push(row);
			else grouped.set(row.date, [row]);
		}
		return grouped;
	});

	function rowsFor(date: string): UsageChartRow[] {
		return rowsByDay.get(date) ?? [];
	}
</script>

<section class="space-y-3" aria-labelledby={`${chartId}-title`}>
	<div class="flex flex-wrap items-baseline justify-between gap-2">
		<h2 id={`${chartId}-title`} class="text-xl font-semibold">Token volume per day</h2>
		<p class="text-xs text-base-content/60">
			Stacked {axis === 'components' ? 'components' : `by ${usageAxisTitle(axis).toLowerCase()}`} ·
			local days in {report.timezone}
		</p>
	</div>

	<p id={`${chartId}-summary`} class="sr-only">{summary}</p>

	<div class="w-full" data-testid="usage-chart" data-usage-axis={axis}>
		<Chart
			data={rows}
			x="date"
			y="tokens"
			c="series"
			cDomain={seriesKeys}
			cRange={seriesColors}
			bandPadding={0.24}
			height={240}
			padding={{ top: 10, right: 8, bottom: 26, left: 52 }}
		>
			{#snippet children({ context })}
				<Svg
					title="Stacked daily token volume. The daily table below holds the same numbers."
					aria-describedby={`${chartId}-summary`}
				>
					{#snippet defs()}
						<pattern
							id={`${chartId}-unmeasured`}
							patternUnits="userSpaceOnUse"
							width="6"
							height="6"
						>
							<path
								d="M0,6 L6,0"
								stroke="var(--color-warning)"
								stroke-width="1.2"
								fill="none"
							/>
						</pattern>
						<pattern
							id={`${chartId}-incomplete`}
							patternUnits="userSpaceOnUse"
							width="6"
							height="6"
						>
							<circle cx="3" cy="3" r="1.2" fill="var(--color-info)" />
						</pattern>
					{/snippet}

					<Axis placement="left" format={(value: number) => formatTokenCount(value)} grid />
					<Axis
						placement="bottom"
						format={(value: string) => value.slice(5)}
						tickSpacing={64}
						tickOcclusion
					/>

					<Bars
						stroke="none"
						tooltip
						onBarClick={(_event: MouseEvent, { data }: { data: UsageChartRow }) =>
							onSelectDay?.(data.date)}
					/>

					{#each flagged as day (day.date)}
						<rect
							data-usage-state={usageDayState(day)}
							x={context.xScale(day.date)}
							y={0}
							width={Math.max(context.xScale.bandwidth?.() ?? 0, 3)}
							height={5}
							fill={`url(#${chartId}-${usageDayState(day)})`}
						/>
					{/each}

					{#if onSelectDay}
						<!-- The pointer path is the library's click callback on the bars; this
						     is the keyboard path. A transparent rect per day column is a real
						     control with a name that states the action, and `pointer-events:
						     none` keeps it from stealing the tooltip or the bar's own click. -->
						{#each report.days as day (day.date)}
							<rect
								class="usage-day-target"
								role="button"
								tabindex="0"
								aria-label={selectedDay === day.date
									? `Show the wider range instead of ${day.date}`
									: `Narrow the range to ${day.date}`}
								aria-pressed={selectedDay === day.date}
								x={context.xScale(day.date)}
								y={0}
								width={Math.max(context.xScale.bandwidth?.() ?? 0, 3)}
								height={context.height}
								fill="transparent"
								style="pointer-events: none;"
								onkeydown={(event) => {
									if (event.key !== 'Enter' && event.key !== ' ') return;
									event.preventDefault();
									onSelectDay(day.date);
								}}
							/>
						{/each}
					{/if}
				</Svg>

				<Tooltip.Root>
					{#snippet children({ data })}
						{#if data}
							<div class="space-y-1">
								<p class="font-semibold">{data.date}</p>
								<ul class="space-y-0.5">
									{#each rowsFor(data.date) as row (row.series)}
										{#if row.tokens > 0}
											<li class="flex justify-between gap-4">
												<span class="text-base-content/70">{row.label}</span>
												<span class="font-mono">{formatTokenCount(row.tokens)}</span>
											</li>
										{/if}
									{/each}
									<li
										class="flex justify-between gap-4 border-t border-base-300 pt-0.5 font-semibold"
									>
										<span>Total</span>
										<span class="font-mono">{formatTokenCount(data.dayTotal)}</span>
									</li>
								</ul>
								{#if data.unmeasured}
									<p class="text-xs text-base-content/70">
										Executions this day reported no usage.
									</p>
								{:else if data.incomplete}
									<p class="text-xs text-base-content/70">
										This day has an execution still in flight.
									</p>
								{/if}
							</div>
						{/if}
					{/snippet}
				</Tooltip.Root>
			{/snippet}
		</Chart>
	</div>

	<ul class="flex flex-wrap gap-x-4 gap-y-2" aria-label="Chart series">
		{#each series as entry (entry.id)}
			<li class="flex items-center gap-2 text-sm">
				<span class={`size-3 rounded-sm ${entry.swatch}`}></span>
				<span>{entry.label}</span>
			</li>
		{/each}
	</ul>

	<p class="text-xs text-base-content/60">
		A hatched strip marks a day whose executions reported no usage; a dotted strip marks a day
		with an execution still in flight. The daily table below is the authoritative reading of the
		same numbers.
		{#if onSelectDay}
			Select a day column to narrow the range to that day; the range you were reading comes back
			when you remove the day chip.
		{/if}
	</p>
</section>

<style>
	/* The day columns are transparent so the bars and the tooltip stay in front;
	       a focused column needs a visible outline of its own. */
	.usage-day-target:focus-visible {
		outline: none;
		stroke: var(--color-primary);
		stroke-width: 2;
	}
</style>
