import { readFileSync, readdirSync } from 'node:fs';
import { join, relative } from 'node:path';
import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import UsageChart, {
	USAGE_COMPONENTS,
	usageChartRows,
	usageChartSummary,
	usagePaletteIndex,
	usageSeriesColor,
	usageSeriesSwatch,
	usageDayState
} from './UsageChart.svelte';
import { usageDay, usageReport } from '../../tests/fixtures';
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

		const rows = usageChartRows(days);
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
