import { readFileSync, readdirSync } from 'node:fs';
import { join, relative } from 'node:path';
import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import UsageChart, {
	USAGE_COMPONENTS,
	USAGE_OTHER_SERIES,
	USAGE_PALETTE,
	usageChartRows,
	usageChartSeries,
	usageChartSummary,
	usagePaletteIndex,
	usageSeriesColor,
	usageSeriesSwatch,
	usageDayState,
	usageDayStateLabel
} from './UsageChart.svelte';
import { usageAxisEntry, usageDay, usageReport } from '../../tests/fixtures';
import type { UsageDay } from '$lib/api/types';

/**
 * Every source file under `web/src`, read through Vite rather than the
 * filesystem: the guard is about the modules this app compiles, and a raw glob
 * keeps the test free of a Node type dependency the console does not carry.
 */
const srcRoot = join(process.cwd(), 'src');

/** Every source file under `web/src`, as paths relative to that root. */
function sourceFiles(dir = srcRoot): string[] {
	const found: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const path = join(dir, entry.name);
		if (entry.isDirectory()) found.push(...sourceFiles(path));
		else if (entry.name.endsWith('.ts') || entry.name.endsWith('.svelte')) {
			found.push(relative(srcRoot, path));
		}
	}
	return found;
}

function sourceOf(file: string): string {
	return readFileSync(join(srcRoot, file), 'utf8');
}

const unmeasuredDay = () =>
	usageDay({
		date: '2026-09-26',
		executions: 1,
		reportedExecutions: 0,
		distinctTrails: 1,
		unmeasured: true,
		tokens: {
			inputTokens: 0,
			outputTokens: 0,
			reasoningTokens: 0,
			cacheReadTokens: 0,
			cacheWriteTokens: 0,
			totalTokens: 0
		},
		recordedCostUsd: 0,
		costReportedExecutions: 0
	});

describe('usage chart data', () => {
	it('stacks the five components to each day’s reported total', () => {
		const days: UsageDay[] = [
			usageDay(),
			usageDay({
				date: '2026-09-27',
				executions: 3,
				reportedExecutions: 2,
				distinctTrails: 2,
				incomplete: true,
				tokens: {
					inputTokens: 10,
					outputTokens: 20,
					reasoningTokens: 30,
					cacheReadTokens: 40,
					cacheWriteTokens: 50,
					totalTokens: 150
				}
			}),
			unmeasuredDay()
		];

		const rows = usageChartRows(usageReport({ days }));
		for (const day of days) {
			const stacked = rows
				.filter((row) => row.date === day.date)
				.reduce((sum, row) => sum + row.tokens, 0);
			expect(stacked).toBe(day.tokens.totalTokens);
			// Every component is present, including the zeroes, so the sum is the
			// total rather than the total of whatever happened to be non-zero.
			expect(rows.filter((row) => row.date === day.date)).toHaveLength(USAGE_COMPONENTS.length);
		}
	});

	it('maps a series key to a stable theme token, not a literal colour', () => {
		expect(usagePaletteIndex('input')).toBe(0);
		expect(usagePaletteIndex('input')).toBe(usagePaletteIndex('input'));
		// A key outside the component order (a later breakdown axis) hashes to the
		// same slot every time, so a model keeps its colour across reloads.
		const model = 'opencode/space-bunny-free';
		expect(usagePaletteIndex(model)).toBe(usagePaletteIndex(model));
		expect(usagePaletteIndex(model)).toBeLessThan(5);
		expect(usageSeriesColor('cache-read')).toMatch(/^var\(--color-[a-z-]+\)$/);
		expect(usageSeriesSwatch('cache-read')).toMatch(/^bg-[a-z]+$/);
	});

	it('stacks a breakdown axis the report already holds, with labels beside the keys', () => {
		const axisTokens = (input: number) => ({
			inputTokens: input,
			outputTokens: 0,
			reasoningTokens: 0,
			cacheReadTokens: 0,
			cacheWriteTokens: 0,
			totalTokens: input
		});
		const days: UsageDay[] = [
			usageDay({
				date: '2026-09-25',
				tokens: axisTokens(300),
				breakdown: {
					adapter: [
						usageAxisEntry({ key: 'opencode', label: 'opencodev2', tokens: axisTokens(200) }),
						usageAxisEntry({ key: 'cursor', tokens: axisTokens(100) })
					],
					model: [
						usageAxisEntry({
							key: 'anthropic/claude-sonnet',
							label: 'sonnet',
							tokens: axisTokens(300)
						})
					],
					bee: [usageAxisEntry({ key: 'builder', tokens: axisTokens(300) })],
					purpose: [usageAxisEntry({ key: 'work', tokens: axisTokens(300) })]
				}
			})
		];
		const report = usageReport({ days });

		const adapters = usageChartSeries(report, 'adapter');
		expect(adapters.map((entry) => entry.label)).toEqual(['opencode · opencodev2', 'cursor']);
		const rows = usageChartRows(report, 'adapter');
		expect(rows.reduce((sum, row) => sum + row.tokens, 0)).toBe(300);

		const models = usageChartSeries(report, 'model');
		expect(models[0].label).toBe('anthropic/claude-sonnet · sonnet');
	});

	it('bounds axis series by the palette and folds the rest into other', () => {
		const tokens = (input: number) => ({
			inputTokens: input,
			outputTokens: 0,
			reasoningTokens: 0,
			cacheReadTokens: 0,
			cacheWriteTokens: 0,
			totalTokens: input
		});
		const bees = Array.from({ length: 7 }, (_, index) =>
			usageAxisEntry({
				key: `bee-${index}`,
				executions: 1,
				tokens: tokens(100 - index),
				recordedCostUsd: 0,
				costReportedExecutions: 0
			})
		);
		const day = usageDay({
			executions: 7,
			reportedExecutions: 7,
			tokens: tokens(7 * 100 - 21),
			recordedCostUsd: 0,
			costReportedExecutions: 0,
			breakdown: {
				adapter: bees.map((entry) => ({ ...entry, key: `adapter-${entry.key}` })),
				model: bees.map((entry) => ({ ...entry, key: `model-${entry.key}` })),
				bee: bees,
				purpose: bees.map((entry) => ({ ...entry, key: `purpose-${entry.key}` }))
			}
		});
		const report = usageReport({ days: [day] });

		const series = usageChartSeries(report, 'bee');
		expect(series).toHaveLength(USAGE_PALETTE.length);
		expect(series.at(-1)?.id).toBe(USAGE_OTHER_SERIES);
		expect(series.at(-1)?.label).toBe('Other (3)');
		// The chart's stack is still the day's total; the table is what lists
		// every key.
		const rows = usageChartRows(report, 'bee', series);
		expect(rows).toHaveLength(USAGE_PALETTE.length);
		expect(rows.reduce((sum, row) => sum + row.tokens, 0)).toBe(day.tokens.totalTokens);
		expect(report.breakdown.bee).toHaveLength(7);
	});

	it('summarises the period for assistive technology', () => {
		const summary = usageChartSummary(
			usageReport({ days: [usageDay(), unmeasuredDay()] })
		);
		expect(summary).toContain('from 2026-09-25 to 2026-09-26');
		expect(summary).toContain('2.1k tokens in total');
		expect(summary).toContain('1 day is unmeasured');
		expect(summary).toContain('daily table');
	});
});

describe('usage chart', () => {
	it('renders one stack segment per component per day with themed fills', () => {
		const days = [usageDay(), unmeasuredDay()];
		const { container } = render(UsageChart, { report: usageReport({ days }) });

		const bars = [...container.querySelectorAll('.lc-bar')];
		expect(bars).toHaveLength(days.length * USAGE_COMPONENTS.length);
		for (const bar of bars) {
			// A token reference, not a resolved colour: the theme switch repaints
			// the same mark without re-rendering it.
			expect(bar.getAttribute('fill')).toMatch(/^var\(--color-[a-z-]+\)$/);
		}
	});

	it('has a legend naming every series', () => {
		render(UsageChart, { report: usageReport() });
		const legend = screen.getByRole('list', { name: 'Chart series' });
		for (const component of USAGE_COMPONENTS) {
			expect(within(legend).getByText(component.label)).toBeInTheDocument();
		}
	});

	it('marks unmeasured and incomplete days by pattern as well as tone', () => {		const days = [
			usageDay(),
			unmeasuredDay(),
			usageDay({ date: '2026-09-27', incomplete: true })
		];
		const { container } = render(UsageChart, {
			report: usageReport({ days })
		});

		const unmeasured = container.querySelector('[data-usage-state="unmeasured"]');
		const incomplete = container.querySelector('[data-usage-state="incomplete"]');
		expect(unmeasured).not.toBeNull();
		expect(incomplete).not.toBeNull();
		expect(unmeasured?.getAttribute('fill')).toMatch(/^url\(#.*unmeasured\)$/);
		expect(incomplete?.getAttribute('fill')).toMatch(/^url\(#.*incomplete\)$/);
		// A colour alone would not say which state the day is in; the note does.
		expect(screen.getByText(/hatched strip marks a day/i)).toBeInTheDocument();
		expect(usageDayState(unmeasuredDay())).toBe('unmeasured');

		// The strip is a mark, not a name: the flagged days are also listed by
		// date, with every state a day carries.
		const flagged = container.querySelector('[data-usage-flagged-days]');
		expect(flagged?.textContent).toContain('2026-09-26');
		expect(flagged?.textContent).toContain('unmeasured');
		expect(flagged?.textContent).toContain('2026-09-27');
		expect(flagged?.textContent).toContain('incomplete');
	});

	it('names both states of a day that is unmeasured and still in flight', () => {
		const both = usageDay({
			date: '2026-09-28',
			executions: 1,
			reportedExecutions: 0,
			unmeasured: true,
			incomplete: true,
			tokens: {
				inputTokens: 0,
				outputTokens: 0,
				reasoningTokens: 0,
				cacheReadTokens: 0,
				cacheWriteTokens: 0,
				totalTokens: 0
			},
			recordedCostUsd: 0,
			costReportedExecutions: 0
		});
		const { container } = render(UsageChart, { report: usageReport({ days: [both] }) });
		expect(usageDayStateLabel(both)).toBe('unmeasured, incomplete');
		expect(container.querySelector('[data-usage-flagged-days]')?.textContent).toContain(
			'unmeasured, incomplete'
		);
	});

	it('offers one named day control per column only when a selection is accepted', () => {
		const days = [usageDay(), usageDay({ date: '2026-09-26' })];

		const readOnly = render(UsageChart, { report: usageReport({ days }) });
		expect(screen.queryByRole('button', { name: /Narrow the range/ })).not.toBeInTheDocument();
		readOnly.unmount();

		render(UsageChart, {
			report: usageReport({ days }),
			onSelectDay: () => {},
			selectedDay: '2026-09-26'
		});
		expect(
			screen.getByRole('button', { name: 'Narrow the range to 2026-09-25' })
		).toBeInTheDocument();
		expect(
			screen.getByRole('button', { name: 'Show the wider range instead of 2026-09-26' })
		).toHaveAttribute('aria-pressed', 'true');
	});
});

describe('chart boundaries', () => {
	it('imports the chart library in the wrapper and nowhere else', () => {
		const importers = sourceFiles().filter((file) =>
			/from\s+['"]layerchart(?:\/[^'"]*)?['"]/.test(sourceOf(file))
		);
		expect(importers).toEqual(['lib/components/UsageChart.svelte']);
	});

	it('keeps raw colour literals out of the chart component', () => {
		const source = sourceOf('lib/components/UsageChart.svelte');
		const literals = [/#[0-9a-fA-F]{3,8}\b/, /\brgba?\(/, /\bhsla?\(/, /\boklch\(/, /\bcolor:/];
		for (const pattern of literals) {
			expect(source).not.toMatch(pattern);
		}
	});

	it('themes through the one DaisyUI bridge import in the global stylesheet', () => {
		const appCss = sourceOf('app.css');
		expect(appCss.match(/@import\s+["']layerchart\/daisyui-5\.css["']/g)).toHaveLength(1);
		// No component imports a LayerChart stylesheet of its own; the one import
		// is what maps surfaces, text, and the tooltip root onto the theme.
		const elsewhere = sourceFiles().filter((file) =>
			/layerchart\/[^'"]*\.css/.test(sourceOf(file))
		);
		expect(elsewhere).toEqual([]);
	});
});
