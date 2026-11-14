import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Usage from './+page.svelte';
import { createUsageStore, type UsageStore } from '$lib/stores/usage.svelte';
import { usageAxisEntry, usageDay, usageReport, usageTrailEntry, usageTrails } from '../../tests/fixtures';
import type { UsageReport } from '$lib/api/types';
import type { UsageAxisChoice, UsageFilters } from '$lib/usage-view';

const fixedNow = () => new Date(2026, 8, 30, 12, 0, 0);

function harness(
	report: UsageReport = usageReport(),
	initial: { filters?: Partial<UsageFilters>; axis?: UsageAxisChoice } = {}
): {
	store: UsageStore;
	fetchUsage: ReturnType<typeof vi.fn>;
} {
	const fetchUsage = vi.fn(async () => report);
	const store = createUsageStore({
		fetchUsage,
		now: fixedNow,
		timezone: 'UTC',
		initialFilters: initial.filters,
		initialAxis: initial.axis
	});
	return { store, fetchUsage };
}

describe('usage page', () => {
	it('renders the headline as token volume and never as money', async () => {
		const { store } = harness(
			usageReport({
				days: [usageDay({ recordedCostUsd: 12.5, costReportedExecutions: 2 })]
			})
		);
		render(Usage, { store });
		await waitFor(() => expect(screen.getByRole('region', { name: 'Token volume' })).toBeInTheDocument());

		const headline = screen.getByRole('region', { name: 'Token volume' });
		expect(headline.textContent).not.toContain('$');
		for (const label of ['Input', 'Output', 'Reasoning', 'Cache read', 'Cache write', 'Total']) {
			expect(within(headline).getByText(label)).toBeInTheDocument();
		}
		expect(within(headline).getByText('Executions')).toBeInTheDocument();
		expect(within(headline).getByText('Trails')).toBeInTheDocument();
		expect(within(headline).getByText('Coverage')).toBeInTheDocument();
		expect(within(headline).getByText('2 of 2 reported')).toBeInTheDocument();

		// The daily table is still the authoritative place for recorded cost.
		const table = within(screen.getByLabelText('Daily usage')).getByRole('table');
		expect(within(table).getByText('$12.50')).toBeInTheDocument();
		expect(within(table).getAllByText('2').length).toBeGreaterThan(0);
	});

	it('distinguishes a day with no executions from a day whose executions reported nothing', async () => {
		const { store } = harness(
			usageReport({
				days: [
					usageDay({ date: '2026-09-25', executions: 2, reportedExecutions: 2 }),
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
					}),
					usageDay({
						date: '2026-09-27',
						executions: 0,
						reportedExecutions: 0,
						distinctTrails: 0,
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
					})
				]
			})
		);
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		const table = within(screen.getByLabelText('Daily usage')).getByRole('table');
		expect(within(table).getByText('reported')).toBeInTheDocument();
		expect(within(table).getByText('unmeasured')).toBeInTheDocument();
		expect(within(table).getByText('no executions')).toBeInTheDocument();
	});

	it('reads once on mount and starts no poller', async () => {
		const { store, fetchUsage } = harness();
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		// The store owns the only read path and it has no timer; see the store
		// suite for the clock assertion. The page mounts it once.
		expect(fetchUsage).toHaveBeenCalledTimes(1);
	});

	it('draws the report the page already holds, with no second read', async () => {
		const { store, fetchUsage } = harness();
		render(Usage, { store });
		const chart = await screen.findByRole('region', { name: 'Token volume per day' });

		// The chart is the page's report, not a reader of its own.
		expect(within(chart).getByText(/from 2026-09-25 to 2026-09-25/)).toBeInTheDocument();
		expect(within(chart).getByText(/2\.1k tokens in total/)).toBeInTheDocument();
		expect(fetchUsage).toHaveBeenCalledTimes(1);

		// The stack and the daily table report the same total for the day.
		const table = screen.getByLabelText('Daily usage');
		expect(within(table).getByText('2.1k')).toBeInTheDocument();
	});

	it('repaints the chart on a theme switch without a reload or a fetch', async () => {
		const { store, fetchUsage } = harness();
		const { container } = render(Usage, { store });
		await waitFor(() => expect(container.querySelector('.lc-bar')).not.toBeNull());

		const fills = [...container.querySelectorAll('.lc-bar')].map((bar) =>
			bar.getAttribute('fill')
		);
		expect(fills.length).toBeGreaterThan(0);
		// Marks carry token references; the theme, not the component, resolves them.
		for (const fill of fills) expect(fill).toMatch(/^var\(--color-[a-z-]+\)$/);

		document.documentElement.dataset.theme = 'catppuccin-mocha';
		await waitFor(() =>
			expect(document.documentElement.dataset.theme).toBe('catppuccin-mocha')
		);
		expect(
			[...container.querySelectorAll('.lc-bar')].map((bar) => bar.getAttribute('fill'))
		).toEqual(fills);
		expect(fetchUsage).toHaveBeenCalledTimes(1);

		delete document.documentElement.dataset.theme;
	});

	it('refreshes on request and reads the new range when the control changes', async () => {
		const { store, fetchUsage } = harness();
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		// The refresh control is the bypass path: the server is asked to scan
		// again rather than answer from its short-lived cache.
		expect(fetchUsage.mock.calls[1][0]).toMatchObject({ refresh: true });

		await userEvent.click(screen.getByRole('button', { name: '7 days' }));
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-24',
			until: '2026-10-01',
			timezone: 'UTC',
			adapter: [],
			model: [],
			bee: [],
			purpose: []
		});
	});

	it('shows the scan time the report came from', async () => {
		const report = usageReport({ observedAt: '2026-09-30T18:00:00Z' });
		const { store } = harness(report);
		const { container } = render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		// A cached answer is explicit rather than hidden: the page names the
		// scan behind the numbers it is showing.
		const observed = container.querySelector('[data-usage-observed-at]');
		expect(observed).not.toBeNull();
		expect(observed).toHaveAttribute('data-usage-observed-at', '2026-09-30T18:00:00Z');
		expect(observed?.textContent).toContain('observed');
	});

	it('shows the server refusal and keeps the last good report', async () => {
		const good = usageReport();
		const fetchUsage = vi
			.fn<() => Promise<UsageReport>>()
			.mockResolvedValueOnce(good)
			.mockRejectedValueOnce(new Error('until 2026-10-01 must be after since 2026-10-02'));
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
		await waitFor(() =>
			expect(screen.getByRole('alert')).toHaveTextContent('must be after since')
		);
		// The numbers an operator was reading are still real ones.
		expect(screen.getByLabelText('Daily usage')).toBeInTheDocument();
	});
});

/** Token components for a total that is all input, so a share is easy to read. */
function inputTokens(total: number) {
	return {
		inputTokens: total,
		outputTokens: 0,
		reasoningTokens: 0,
		cacheReadTokens: 0,
		cacheWriteTokens: 0,
		totalTokens: total
	};
}

describe('usage page breakdown', () => {
	it('switches the chart stack from the loaded report without a second read', async () => {
		const day = usageDay({
			tokens: inputTokens(400),
			breakdown: {
				adapter: [usageAxisEntry({ key: 'opencode', tokens: inputTokens(400) })],
				model: [
					usageAxisEntry({ key: 'anthropic/claude-sonnet', label: 'sonnet', tokens: inputTokens(300) }),
					usageAxisEntry({ key: 'openai/gpt-5', tokens: inputTokens(100) })
				],
				bee: [usageAxisEntry({ key: 'builder', tokens: inputTokens(400) })],
				purpose: [usageAxisEntry({ key: 'work', tokens: inputTokens(400) })]
			}
		});
		const { store, fetchUsage } = harness(usageReport({ days: [day] }));
		const { container } = render(Usage, { store });
		await waitFor(() => expect(container.querySelector('.lc-bar')).not.toBeNull());

		// The component stack is the default: five series per day.
		expect(container.querySelector('[data-testid="usage-chart"]')?.getAttribute('data-usage-axis')).toBe(
			'components'
		);
		expect(container.querySelectorAll('.lc-bar')).toHaveLength(5);

		await userEvent.click(screen.getByRole('button', { name: 'Model' }));
		await waitFor(() =>
			expect(container.querySelectorAll('.lc-bar')).toHaveLength(2)
		);
		expect(container.querySelector('[data-testid="usage-chart"]')?.getAttribute('data-usage-axis')).toBe(
			'model'
		);
		const legend = screen.getByRole('list', { name: 'Chart series' });
		expect(legend.textContent).toContain('anthropic/claude-sonnet · sonnet');
		expect(legend.textContent).toContain('openai/gpt-5');

		// The axis came out of the report already on screen.
		expect(fetchUsage).toHaveBeenCalledTimes(1);
	});

	it('ranks the breakdown table by the selected axis and shows each share', async () => {
		const day = usageDay({
			tokens: inputTokens(1000),
			breakdown: {
				adapter: [usageAxisEntry({ key: 'opencode', tokens: inputTokens(1000) })],
				model: [
					usageAxisEntry({ key: 'big-model', tokens: inputTokens(750) }),
					usageAxisEntry({ key: 'small-model', tokens: inputTokens(250) })
				],
				bee: [usageAxisEntry({ key: 'builder', tokens: inputTokens(1000) })],
				purpose: [usageAxisEntry({ key: 'work', tokens: inputTokens(1000) })]
			}
		});
		const { store } = harness(usageReport({ days: [day] }));
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Model' }));
		const breakdown = await screen.findByRole('region', { name: 'Period breakdown by model' });
		const table = within(breakdown).getByRole('table');
		const rows = within(table).getAllByRole('row').slice(1);
		expect(rows[0].textContent).toContain('big-model');
		expect(rows[0].textContent).toContain('750');
		expect(rows[0].textContent).toContain('75.0%');
		expect(rows[1].textContent).toContain('small-model');
		expect(rows[1].textContent).toContain('25.0%');
	});

	it('draws the top keys and an other series while the table lists every key', async () => {
		const bees = Array.from({ length: 7 }, (_, index) =>
			usageAxisEntry({
				key: `bee-${index}`,
				executions: 1,
				tokens: inputTokens(100 - index),
				recordedCostUsd: 0,
				costReportedExecutions: 0
			})
		);
		const day = usageDay({
			executions: 7,
			reportedExecutions: 7,
			tokens: inputTokens(7 * 100 - 21),
			recordedCostUsd: 0,
			costReportedExecutions: 0,
			breakdown: {
				adapter: [usageAxisEntry({ key: 'opencode', tokens: inputTokens(7 * 100 - 21) })],
				model: [usageAxisEntry({ key: 'a-model', tokens: inputTokens(7 * 100 - 21) })],
				bee: bees,
				purpose: [usageAxisEntry({ key: 'work', tokens: inputTokens(7 * 100 - 21) })]
			}
		});
		const { store } = harness(usageReport({ days: [day] }));
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Bee' }));
		const chart = await screen.findByTestId('usage-chart');
		const legend = screen.getByRole('list', { name: 'Chart series' });
		expect(chart).toBeInTheDocument();
		expect(legend.textContent).toContain('Other (3)');

		const breakdown = await screen.findByRole('region', { name: 'Period breakdown by bee' });
		const table = within(breakdown).getByRole('table');
		expect(within(table).getAllByRole('row')).toHaveLength(8); // header plus seven keys
	});
});

describe('usage page component breakdown', () => {
	it('ranks the default component stack with shares and no invented executions', async () => {
		const { store } = harness();
		render(Usage, { store });
		const breakdown = await screen.findByRole('region', {
			name: 'Period breakdown by component'
		});
		const table = within(breakdown).getByRole('table');
		const rows = within(table).getAllByRole('row').slice(1);
		expect(rows).toHaveLength(5);
		// Input is the largest component in the fixture (1200 of 2060 tokens).
		expect(rows[0].textContent).toContain('Input');
		expect(rows[0].textContent).toContain('58.3%');
		// A component has no execution count or recorded cost of its own.
		expect(rows[0].textContent).toContain('—');
	});
});

describe('usage page top trails', () => {
	it('links each trail to its existing detail and falls back to the id', async () => {
		const day = usageDay({ tokens: inputTokens(800) });
		const { store } = harness(
			usageReport({
				days: [day],
				trails: usageTrails({
					measuredTrails: 2,
					entries: [
						usageTrailEntry({
							traceId: 'trace-titled',
							title: 'Fix the cache split',
							executions: 3,
							reportedExecutions: 2,
							tokens: inputTokens(600),
							recordedCostUsd: 1.5,
							costReportedExecutions: 1
						}),
						usageTrailEntry({
							traceId: 'trace-untitled',
							title: undefined,
							executions: 1,
							reportedExecutions: 1,
							tokens: inputTokens(200),
							recordedCostUsd: 0,
							costReportedExecutions: 0
						})
					]
				})
			})
		);
		render(Usage, { store });
		const region = await screen.findByRole('region', { name: 'Top trails' });
		const table = within(region).getByRole('table');
		const rows = within(table).getAllByRole('row').slice(1);

		const titled = within(rows[0]).getByRole('link', { name: 'Fix the cache split' });
		expect(titled).toHaveAttribute('href', '/traces/trace-titled');
		expect(rows[0].textContent).toContain('3');
		expect(rows[0].textContent).toContain('600');
		// Recorded cost carries the count of executions that reported it.
		expect(rows[0].textContent).toContain('$1.50 (1)');

		// A trail with no recorded title is still a row, named by its id.
		const untitled = within(rows[1]).getByRole('link', { name: 'trace-untitled' });
		expect(untitled).toHaveAttribute('href', '/traces/trace-untitled');
		expect(rows[1].textContent).toContain('—');
	});

	it('states how many trails remain outside the table so the rows reconcile with the period', async () => {
		const day = usageDay({ tokens: inputTokens(1000) });
		const { store } = harness(
			usageReport({
				days: [day],
				trails: usageTrails({
					measuredTrails: 4,
					outsideTrails: 2,
					outsideTokens: 100,
					entries: [
						usageTrailEntry({ traceId: 'trace-big', tokens: inputTokens(600) }),
						usageTrailEntry({ traceId: 'trace-mid', tokens: inputTokens(300) })
					]
				})
			})
		);
		render(Usage, { store });
		const region = await screen.findByRole('region', { name: 'Top trails' });
		const table = within(region).getByRole('table');
		expect(within(table).getAllByRole('row')).toHaveLength(3); // header plus two rows

		const note = region.querySelector('p')?.textContent ?? '';
		expect(note).toContain('The rows above account for 900 tokens');
		expect(note).toContain('2 measured trails outside this table account for 100');
		// The shown rows plus the stated remainder are the period total.
		expect(note).toContain("the period's 1.0k tokens in total");
	});

	it('renders an empty period without a table and a single trail without a remainder', async () => {
		const emptyDay = usageDay({
			executions: 0,
			reportedExecutions: 0,
			distinctTrails: 0,
			tokens: inputTokens(0),
			recordedCostUsd: 0,
			costReportedExecutions: 0
		});
		const empty = harness(usageReport({ days: [emptyDay] }));
		const first = render(Usage, { store: empty.store });
		const emptyRegion = await screen.findByRole('region', { name: 'Top trails' });
		expect(within(emptyRegion).queryByRole('table')).not.toBeInTheDocument();
		expect(emptyRegion.textContent).toContain('No trail reported usage in this range.');
		first.unmount();

		const single = harness();
		render(Usage, { store: single.store });
		const singleRegion = await screen.findByRole('region', { name: 'Top trails' });
		expect(within(singleRegion).getAllByRole('row')).toHaveLength(2); // header plus one trail
		expect(singleRegion.textContent).toContain('Every trail that reported usage is listed');
	});
});

/** A day whose model axis holds two keys, so a filter has a choice to make. */
function modelDay() {
	return usageDay({
		breakdown: {
			adapter: [usageAxisEntry({ key: 'opencode' })],
			model: [
				usageAxisEntry({ key: 'openai/gpt-5' }),
				usageAxisEntry({ key: 'anthropic/claude-sonnet' })
			],
			bee: [usageAxisEntry({ key: 'builder' })],
			purpose: [usageAxisEntry({ key: 'work' })]
		}
	});
}

describe('usage page filters', () => {
	it('renders active filters as removable chips and offers the values still in view', async () => {
		const { store, fetchUsage } = harness(usageReport({ days: [modelDay()] }), {
			filters: { model: ['openai/gpt-5'] }
		});
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		const chips = screen.getByRole('list', { name: 'Active filters' });
		expect(chips.textContent).toContain('Model: openai/gpt-5');

		// The applied value is a chip, not a second choice in the picker.
		const picker = screen.getByLabelText('Add Model filter') as HTMLSelectElement;
		const offered = within(picker)
			.getAllByRole('option')
			.map((option) => option.textContent?.trim());
		expect(offered).toContain('anthropic/claude-sonnet');
		expect(offered).not.toContain('openai/gpt-5');

		await userEvent.click(
			screen.getByRole('button', { name: 'Remove model filter openai/gpt-5' })
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-01',
			until: '2026-10-01',
			timezone: 'UTC',
			adapter: [],
			model: [],
			bee: [],
			purpose: []
		});
		expect(screen.queryByRole('list', { name: 'Active filters' })).not.toBeInTheDocument();
	});

	it('adds a filter from the picker in one read', async () => {
		const { store, fetchUsage } = harness(usageReport({ days: [modelDay()] }), {
			axis: 'model'
		});
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		await userEvent.selectOptions(
			screen.getByLabelText('Add Model filter'),
			'openai/gpt-5'
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ model: ['openai/gpt-5'] })
		);
		expect(
			screen.getByRole('button', { name: 'Remove model filter openai/gpt-5' })
		).toBeInTheDocument();
	});

	it('shows a distinct empty state when the filters match nothing, and keeps every chip removable', async () => {
		const emptyDay = usageDay({
			executions: 0,
			reportedExecutions: 0,
			distinctTrails: 0,
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
		const { store, fetchUsage } = harness(usageReport({ days: [emptyDay] }), {
			filters: { adapter: ['opencode'], purpose: ['work'] }
		});
		render(Usage, { store });

		expect(
			await screen.findByText('No executions match the active filters')
		).toBeInTheDocument();
		expect(screen.queryByLabelText('Daily usage')).not.toBeInTheDocument();
		expect(screen.queryByRole('region', { name: 'Token volume' })).not.toBeInTheDocument();

		// Every active chip stays removable, including the ones that match nothing.
		expect(
			screen.getByRole('button', { name: 'Remove adapter filter opencode' })
		).toBeInTheDocument();
		expect(
			screen.getByRole('button', { name: 'Remove purpose filter work' })
		).toBeInTheDocument();

		await userEvent.click(
			screen.getByRole('button', { name: 'Clear filters to see the range' })
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ adapter: [], purpose: [] })
		);
		await waitFor(() =>
			expect(screen.getByLabelText('Daily usage')).toBeInTheDocument()
		);
	});

	it('writes the range, axis, and filters to the URL without adding history entries', async () => {
		const { store, fetchUsage } = harness(usageReport({ days: [modelDay()] }));
		const before = window.history.length;
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		// Mount writes the exact range so a copied link names the days on screen.
		expect(new URLSearchParams(window.location.search).get('since')).toBe('2026-09-01');

		await userEvent.click(screen.getByRole('button', { name: 'Model' }));
		expect(new URLSearchParams(window.location.search).get('axis')).toBe('model');
		// Switching the stack is not a read.
		expect(fetchUsage).toHaveBeenCalledTimes(1);

		await userEvent.selectOptions(
			screen.getByLabelText('Add Model filter'),
			'anthropic/claude-sonnet'
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(new URLSearchParams(window.location.search).getAll('model')).toEqual([
			'anthropic/claude-sonnet'
		]);
		// Replace, never push: Back does not walk through the values added.
		expect(window.history.length).toBe(before);
	});

	it('restores the view a link carries', async () => {
		window.history.replaceState(
			null,
			'',
			'/usage?since=2026-09-10&until=2026-09-12&axis=model&model=openai%2Fgpt-5'
		);
		const { store, fetchUsage } = harness(
			usageReport({
				since: '2026-09-10T00:00:00Z',
				until: '2026-09-12T00:00:00Z',
				days: [modelDay()]
			})
		);
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		expect(fetchUsage).toHaveBeenCalledWith({
			since: '2026-09-10',
			until: '2026-09-12',
			timezone: 'UTC',
			adapter: [],
			model: ['openai/gpt-5'],
			bee: [],
			purpose: []
		});
		expect(screen.getByRole('button', { name: 'Model' })).toHaveAttribute(
			'aria-pressed',
			'true'
		);
		expect(
			screen.getByRole('button', { name: 'Remove model filter openai/gpt-5' })
		).toBeInTheDocument();
		expect(
			screen.getByText(/2026-09-10T00:00:00Z to 2026-09-12T00:00:00Z/)
		).toBeInTheDocument();
	});
});

/** Two days, so a day click has a day to choose that is not the only one. */
function twoDays() {
	const breakdown = () => ({
		adapter: [usageAxisEntry({ key: 'opencode' })],
		model: [
			usageAxisEntry({ key: 'openai/gpt-5' }),
			usageAxisEntry({ key: 'anthropic/claude-sonnet' })
		],
		bee: [usageAxisEntry({ key: 'builder' })],
		purpose: [usageAxisEntry({ key: 'work' })]
	});
	return usageReport({
		days: [
			usageDay({ date: '2026-09-25', tokens: inputTokens(300), breakdown: breakdown() }),
			usageDay({ date: '2026-09-26', tokens: inputTokens(700), breakdown: breakdown() })
		]
	});
}

describe('usage page drill-down', () => {
	it('narrows the range to a clicked day and adds a removable day chip', async () => {
		window.history.replaceState(null, '', '/usage');
		const { store, fetchUsage } = harness(twoDays());
		const { container } = render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		const pathBefore = window.location.pathname;
		const historyBefore = window.history.length;
		// The chart's day selection is the library's click callback on the bar:
		// the sixth bar of the component stack is the second day.
		const bars = container.querySelectorAll('.lc-bar');
		expect(bars.length).toBeGreaterThan(5);
		await userEvent.click(bars[5]);

		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-26',
			until: '2026-09-27',
			timezone: 'UTC',
			adapter: [],
			model: [],
			bee: [],
			purpose: []
		});

		const chips = screen.getByRole('list', { name: 'Active filters' });
		expect(chips.textContent).toContain('Day: 2026-09-26');
		expect(
			screen.getByRole('button', { name: 'Remove day filter 2026-09-26' })
		).toBeInTheDocument();

		// The narrowed view is in the URL, and nothing navigated.
		expect(new URLSearchParams(window.location.search).get('day')).toBe('2026-09-26');
		expect(new URLSearchParams(window.location.search).get('since')).toBe('2026-09-01');
		expect(window.location.pathname).toBe(pathBefore);
		expect(window.history.length).toBe(historyBefore);
	});

	it('narrows from the keyboard with a day control that names its action', async () => {
		window.history.replaceState(null, '', '/usage');
		const { store, fetchUsage } = harness(twoDays());
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		const day = screen.getByRole('button', { name: 'Narrow the range to 2026-09-25' });
		day.focus();
		await userEvent.keyboard('{Enter}');

		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ since: '2026-09-25', until: '2026-09-26' })
		);
		// The control reports what a second activation would do.
		expect(
			screen.getByRole('button', { name: 'Show the wider range instead of 2026-09-25' })
		).toBeInTheDocument();
	});

	it('removes the day chip to restore the range without a Back press', async () => {
		window.history.replaceState(null, '', '/usage');
		const { store, fetchUsage } = harness(twoDays());
		const { container } = render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		// Pick the second day from the chart's click callback.
		await userEvent.click(container.querySelectorAll('.lc-bar')[5]);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));

		const historyBefore = window.history.length;
		await userEvent.click(
			screen.getByRole('button', { name: 'Remove day filter 2026-09-26' })
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ since: '2026-09-01', until: '2026-10-01' })
		);
		expect(screen.queryByRole('button', { name: /Remove day filter/ })).not.toBeInTheDocument();
		expect(new URLSearchParams(window.location.search).has('day')).toBe(false);
		expect(window.history.length).toBe(historyBefore);
	});

	it('adds a breakdown row key to the filters and removes it on the next click', async () => {
		window.history.replaceState(null, '', '/usage');
		const { store, fetchUsage } = harness(twoDays(), { axis: 'model' });
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		await userEvent.click(
			screen.getByRole('button', { name: 'Add model filter openai/gpt-5' })
		);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		// The range is untouched: the row narrows the filter set, not the days.
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-01',
			until: '2026-10-01',
			timezone: 'UTC',
			adapter: [],
			model: ['openai/gpt-5'],
			bee: [],
			purpose: []
		});
		expect(
			screen.getByRole('button', { name: 'Remove model filter openai/gpt-5' })
		).toBeInTheDocument();
		// The row itself now offers the other direction, and is a second control
		// from the chip rather than a duplicate name for it.
		const row = screen.getByRole('button', { name: 'Clear model filter openai/gpt-5' });
		expect(row).toHaveAttribute('aria-pressed', 'true');

		await userEvent.click(row);
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ model: [], since: '2026-09-01', until: '2026-10-01' })
		);
	});

	it('activates a breakdown row from the keyboard and filters by its recorded key', async () => {
		window.history.replaceState(null, '', '/usage');
		const day = usageDay({
			breakdown: {
				adapter: [usageAxisEntry({ key: 'opencode', label: 'opencodev2' })],
				model: [usageAxisEntry({ key: 'anthropic/claude-sonnet', label: 'sonnet' })],
				bee: [usageAxisEntry({ key: 'builder' })],
				purpose: [usageAxisEntry({ key: 'work' })]
			}
		});
		const { store, fetchUsage } = harness(usageReport({ days: [day] }), { axis: 'adapter' });
		render(Usage, { store });
		await waitFor(() =>
			expect(
				screen.getByRole('button', { name: 'Add adapter filter opencode' })
			).toBeInTheDocument()
		);

		// The display label carries the implementation; the filter is the key.
		const row = screen.getByRole('button', { name: 'Add adapter filter opencode' });
		row.focus();
		await userEvent.keyboard('{Enter}');

		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ adapter: ['opencode'] })
		);
	});

	it('keeps a drilled view across a reload through the URL', async () => {
		window.history.replaceState(
			null,
			'',
			'/usage?since=2026-09-01&until=2026-10-01&day=2026-09-26&axis=model'
		);
		const { store, fetchUsage } = harness(twoDays());
		render(Usage, { store });
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		expect(fetchUsage).toHaveBeenCalledWith({
			since: '2026-09-26',
			until: '2026-09-27',
			timezone: 'UTC',
			adapter: [],
			model: [],
			bee: [],
			purpose: []
		});
		expect(
			screen.getByRole('button', { name: 'Remove day filter 2026-09-26' })
		).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Model' })).toHaveAttribute(
			'aria-pressed',
			'true'
		);
	});

	it('leaves the page only through a trail link', async () => {
		window.history.replaceState(null, '', '/usage');
		const { store } = harness(twoDays(), { axis: 'model' });
		render(Usage, { store });
		await waitFor(() => expect(screen.getByLabelText('Daily usage')).toBeInTheDocument());

		// Every drill-down control is a button. The trail link is the page's one
		// anchor into another route, and it is outside the narrowed view.
		const drillButtons = screen.getAllByRole('button', { name: /^(Add|Clear) (model|bee|adapter|purpose) filter/ });
		expect(drillButtons.length).toBeGreaterThan(0);
		const trail = screen.getByRole('link', { name: /Currency in the usage report/ });
		expect(trail).toHaveAttribute('href');
	});
});
