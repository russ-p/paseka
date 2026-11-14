<script module lang="ts">
	import { formatTokenCount } from '$lib/format';
	import type { UsageDay, UsageReport } from '$lib/api/types';

	/**
	 * The series the daily chart stacks, in stack order. The order is part of the
	 * contract: the palette is fixed and both the chart and its legend read from
	 * this one list, so a component cannot be coloured in the chart and named
	 * differently in the legend.
	 */
	export const USAGE_COMPONENTS = [
		{ key: 'input', label: 'Input' },
		{ key: 'output', label: 'Output' },
		{ key: 'reasoning', label: 'Reasoning' },
		{ key: 'cache-read', label: 'Cache read' },
		{ key: 'cache-write', label: 'Cache write' }
	] as const;

	export type UsageSeriesKey = (typeof USAGE_COMPONENTS)[number]['key'];

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
	 * order; any other key (the later breakdown axes) hashes to a slot, so the
	 * same key keeps its colour across reloads, ranges, and axes.
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

	/** One stack segment: a day, a component, and its reported volume. */
	export interface UsageChartRow {
		date: string;
		series: UsageSeriesKey;
		label: string;
		tokens: number;
		/** The day's reported total, repeated so a tooltip can name it. */
		dayTotal: number;
		unmeasured: boolean;
		incomplete: boolean;
	}

	/**
	 * Long-form rows for the stacked chart, one per component per day. Everything
	 * comes from the report the page already holds; the chart never reads usage
	 * itself. A row is emitted even for a zero component, so the stack's sum is
	 * the day's total exactly rather than the sum of its non-zero parts.
	 */
	export function usageChartRows(days: UsageDay[]): UsageChartRow[] {
		return days.flatMap((day) =>
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

	/** The summary an assistive technology reads instead of the marks. */
	export function usageChartSummary(report: UsageReport): string {
		const first = report.days.at(0)?.date ?? report.since;
		const last = report.days.at(-1)?.date ?? report.until;
		const period = report.period;
		const unmeasured = report.days.filter((day) => day.unmeasured).length;
		const incomplete = report.days.filter((day) => day.incomplete).length;
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
		const states = [
			unmeasured > 0 ? `${unmeasured} ${unmeasured === 1 ? 'day is' : 'days are'} unmeasured` : '',
			incomplete > 0 ? `${incomplete} ${incomplete === 1 ? 'day is' : 'days are'} incomplete` : ''
		]
			.filter(Boolean)
			.join(' and ');
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

	let { report }: { report: UsageReport } = $props();

	const chartId = $props.id();
	const rows = $derived(usageChartRows(report.days));
	const seriesKeys = USAGE_COMPONENTS.map((component) => component.key);
	const seriesColors = seriesKeys.map((key) => usageSeriesColor(key));
	const flagged = $derived(usageFlaggedDays(report.days));
	const summary = $derived(usageChartSummary(report));
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
			Stacked components · local days in {report.timezone}
		</p>
	</div>

	<p id={`${chartId}-summary`} class="sr-only">{summary}</p>

	<div class="w-full" data-testid="usage-chart">
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

					<Bars stroke="none" tooltip />

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
		{#each USAGE_COMPONENTS as component (component.key)}
			<li class="flex items-center gap-2 text-sm">
				<span class={`size-3 rounded-sm ${usageSeriesSwatch(component.key)}`}></span>
				<span>{component.label}</span>
			</li>
		{/each}
	</ul>

	<p class="text-xs text-base-content/60">
		A hatched strip marks a day whose executions reported no usage; a dotted strip marks a day
		with an execution still in flight. The daily table below is the authoritative reading of the
		same numbers.
	</p>
</section>
