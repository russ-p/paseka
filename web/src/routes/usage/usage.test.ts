import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Usage from './+page.svelte';
import { createUsageStore, type UsageStore } from '$lib/stores/usage.svelte';
import { usageDay, usageReport } from '../../tests/fixtures';
import type { UsageReport } from '$lib/api/types';

const fixedNow = () => new Date(2026, 8, 30, 12, 0, 0);

function harness(report: UsageReport = usageReport()): {
	store: UsageStore;
	fetchUsage: ReturnType<typeof vi.fn>;
} {
	const fetchUsage = vi.fn(async () => report);
	const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
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
		const table = screen.getByRole('table');
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

		await userEvent.click(screen.getByRole('button', { name: '7 days' }));
		await waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-24',
			until: '2026-10-01',
			timezone: 'UTC'
		});
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
