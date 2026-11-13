import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './+page.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { dashboardSummary, insightHighlight, runSummary, traceSummary } from '../../tests/fixtures';
import type { DashboardSummary } from '$lib/api/types';

const goto = vi.fn(async () => {});
vi.mock('$app/navigation', () => ({
	goto: (...args: unknown[]) => goto(...(args as [])),
	// The Failed runs table writes its filter and page to the URL, so a mock this narrow
	// has to restate the stand-in `tests/setup.ts` installs rather than rely on the tests
	// happening not to touch the query.
	replaceState: (url: string | URL, state: App.PageState) =>
		window.history.replaceState(state, '', url)
}));

function harness(summary: DashboardSummary = dashboardSummary()) {
	const store = createConsoleStatusStore({ pollIntervalMs: 0 });
	store.applyChromeFrame({ schemaVersion: 1, runtime: { status: 'running', alive: true, slug: 'paseka' } });
	store.applyDashboard(summary);
	return { store, toasts: createToastStore(0) };
}

afterEach(() => {
	vi.unstubAllGlobals();
	vi.clearAllMocks();
});

describe('dashboard route', () => {
	it('renders the colony snapshot tiles from the dashboard poll', () => {
		const { store, toasts } = harness(
			dashboardSummary({
				activeSessions: 4,
				activeWorktrees: 3,
				recentTraces: [
					traceSummary({ traceId: 'a', hasActive: true }),
					traceSummary({ traceId: 'b', hasActive: true }),
					traceSummary({ traceId: 'c', hasActive: false })
				]
			})
		);
		render(Dashboard, { store, toasts });

		expect(screen.getByRole('heading', { name: 'Dashboard', level: 1 })).toBeInTheDocument();
		expect(screen.getByText('Colony snapshot for paseka, refreshed by the dashboard poll.')).toBeInTheDocument();

		const activeTraces = screen.getByText('Active traces').closest('.stat');
		expect(activeTraces).toHaveTextContent('2');
		expect(screen.getByText('Active sessions').closest('.stat')).toHaveTextContent('4');
		const worktrees = screen.getByText('Active worktrees').closest('.stat');
		expect(worktrees).toHaveTextContent('3');
		// The count is the one place the grid names a route, because the worktrees themselves
		// are listed on their own page and nothing else on the grid leads anywhere.
		expect(within(worktrees as HTMLElement).getByRole('link')).toHaveAttribute(
			'href',
			'/worktrees'
		);
	});

	it('leaves NATS to the topbar instead of repeating it in the stat grid', () => {
		const { store, toasts } = harness();
		render(Dashboard, { store, toasts });

		const grid = document.querySelector('#dashboard-stats');
		expect(within(grid as HTMLElement).queryByText('NATS')).not.toBeInTheDocument();
	});

	it('folds task counts into a wide stat tile, the count badged and the status plain', () => {
		const { store, toasts } = harness(
			dashboardSummary({ taskCounts: { blocked: 1, completed: 29, ready: 1 } })
		);
		render(Dashboard, { store, toasts });

		const tile = document.querySelector('#dashboard-task-counts');
		expect(tile).toHaveClass('stat');
		expect(tile?.className).toContain('lg:col-span-2');

		const grid = document.querySelector('#dashboard-stats') as HTMLElement;
		expect(within(grid).getByText('blocked')).toBeInTheDocument();
		expect(within(grid).getByText('completed')).toBeInTheDocument();
		expect(within(grid).getByText('ready')).toBeInTheDocument();

		const count = tile?.querySelector('.badge');
		expect(count).toHaveTextContent('1');
		expect(count?.className).toContain('badge-error');

		const ready = within(tile as HTMLElement).getByText('ready')
			.nextElementSibling as HTMLElement;
		expect(ready).toHaveTextContent('1');
		expect(ready.className).toContain('badge-success');
	});

	it('lists recent traces, failed runs, and insights', () => {
		const { store, toasts } = harness();
		render(Dashboard, { store, toasts });

		expect(screen.getByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();
		expect(screen.getByText('Refactor the adapter seam')).toBeInTheDocument();

		expect(screen.getByRole('heading', { name: 'Failed runs' })).toBeInTheDocument();
		const table = screen.getByRole('table');
		expect(within(table).getByText('builder')).toBeInTheDocument();
		expect(within(table).getByText('failed')).toBeInTheDocument();

		expect(screen.getByRole('heading', { name: 'Recent insights' })).toBeInTheDocument();
		expect(screen.getByText('The seam is now the only place that knows about providers.')).toBeInTheDocument();
	});

	it('drops runs that succeeded from the failed-runs table', () => {
		const { store, toasts } = harness(
			dashboardSummary({
				recentTraces: [],
				recentInsights: [],
				failedRuns: [
					runSummary({ agentId: 'a', bee: 'builder', state: 'failed' }),
					runSummary({ agentId: 'b', bee: 'guard', state: 'success' })
				]
			})
		);
		render(Dashboard, { store, toasts });

		const table = screen.getByRole('table');
		expect(within(table).getByText('builder')).toBeInTheDocument();
		expect(within(table).queryByText('guard')).not.toBeInTheDocument();
	});

	it('badges a trail row with its open failures and leaves a healed trail quiet', () => {
		const { store, toasts } = harness(
			dashboardSummary({
				recentTraces: [
					traceSummary({
						traceId: 'trace-open',
						title: 'Open trail',
						hasActive: false,
						hasFailures: true,
						openFailures: 2
					}),
					// A record-only trail names the integrity problem in its badge.
					traceSummary({
						traceId: 'trace-record',
						title: 'Record trail',
						hasActive: false,
						hasFailures: true,
						openFailures: 0,
						recordProblem: true
					}),
					// Both axes: the count is the badge.
					traceSummary({
						traceId: 'trace-both',
						title: 'Both trail',
						hasActive: false,
						hasFailures: true,
						openFailures: 1,
						recordProblem: true
					}),
					// Healed: still historically failed, nothing open to act on.
					traceSummary({
						traceId: 'trace-healed',
						title: 'Healed trail',
						hasActive: false,
						hasFailures: true,
						openFailures: 0
					})
				]
			})
		);
		render(Dashboard, { store, toasts });

		const traces = document.querySelector('#dashboard-traces') as HTMLElement;
		expect(within(traces).getByText('2 failures')).toBeInTheDocument();
		expect(within(traces).getByText('record error')).toBeInTheDocument();
		const both = within(traces).getByText('Both trail').closest('li') as HTMLElement;
		expect(within(both).getByText('1 failure')).toBeInTheDocument();
		expect(within(both).queryByText('record error')).not.toBeInTheDocument();
		const healed = within(traces).getByText('Healed trail').closest('li') as HTMLElement;
		expect(within(healed).queryByText(/failure/)).not.toBeInTheDocument();
		expect(within(healed).queryByText('record error')).not.toBeInTheDocument();
	});

	it('renders skeletons before the first poll and empty states after it', () => {
		const store = createConsoleStatusStore({ pollIntervalMs: 0 });
		const { container, unmount } = render(Dashboard, { store, toasts: createToastStore(0) });
		expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0);
		unmount();

		const empty = createConsoleStatusStore({ pollIntervalMs: 0 });
		empty.applyDashboard(
			dashboardSummary({ recentTraces: [], failedRuns: [], recentInsights: [], taskCounts: {} })
		);
		render(Dashboard, { store: empty, toasts: createToastStore(0) });

		expect(screen.getByText('No recent traces.')).toBeInTheDocument();
		expect(screen.getByText('No failed runs.')).toBeInTheDocument();
		expect(screen.getByText('No recent insights.')).toBeInTheDocument();
		expect(document.querySelector('#dashboard-task-counts')).toHaveTextContent('None');
		expect(container.querySelectorAll('.skeleton').length).toBe(0);
	});

	it('surfaces a dashboard error as an alert', () => {
		const store = createConsoleStatusStore({ pollIntervalMs: 0 });
		store.applyChromeFrame({ schemaVersion: 1 });
		store.applyChromeFrame({ schemaVersion: 9 });
		render(Dashboard, { store, toasts: createToastStore(0) });

		expect(screen.getByRole('alert')).toHaveTextContent('unsupported chrome schema: 9');
	});

	it('opens the cue dialog from the quick action and refreshes after publishing', async () => {
		const user = userEvent.setup();
		const loadDashboard = vi.fn(async () => dashboardSummary());
		vi.stubGlobal(
			'fetch',
			vi.fn(async (input: RequestInfo | URL) => {
				const url = String(input);
				if (url === '/api/cues') {
					return new Response(JSON.stringify([{ id: 'ship', description: 'Ship it' }]));
				}
				if (url === '/api/cues/ship/run') {
					return new Response(JSON.stringify({ traceId: 'trace-new' }));
				}
				return new Response('not found', { status: 404 });
			})
		);

		const store = createConsoleStatusStore({ loadDashboard, pollIntervalMs: 0 });
		store.applyDashboard(dashboardSummary());
		const toasts = createToastStore(0);
		render(Dashboard, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Run cue' }));
		await screen.findByLabelText('Cue');
		expect(screen.getByRole('dialog', { name: 'Run cue' })).toBeInTheDocument();

		await user.type(screen.getByLabelText('Text'), 'Ship the release');
		await user.click(screen.getByRole('button', { name: 'Publish' }));

		await waitFor(() => expect(loadDashboard).toHaveBeenCalledTimes(1));
		expect(toasts.items).toHaveLength(1);
		expect(toasts.items[0].message).toBe('Cue published — trace trace-new');
	});

	it('offers the cued trail in the toast rather than navigating to it', async () => {
		const user = userEvent.setup();
		vi.stubGlobal(
			'fetch',
			vi.fn(async (input: RequestInfo | URL) => {
				const url = String(input);
				if (url === '/api/cues') {
					return new Response(JSON.stringify([{ id: 'ship', description: 'Ship it' }]));
				}
				if (url === '/api/cues/ship/run') {
					return new Response(JSON.stringify({ traceId: 'trace-new' }));
				}
				return new Response('not found', { status: 404 });
			})
		);

		const store = createConsoleStatusStore({ loadDashboard: async () => dashboardSummary(), pollIntervalMs: 0 });
		store.applyDashboard(dashboardSummary());
		const toasts = createToastStore(0);
		render(Dashboard, { store, toasts });

		await user.click(screen.getByRole('button', { name: 'Run cue' }));
		await user.type(await screen.findByLabelText('Text'), 'Ship the release');
		await user.click(screen.getByRole('button', { name: 'Publish' }));

		await waitFor(() => expect(toasts.items[0].action?.label).toBe('Open trail'));

		// A standing cue continues a trail the operator may already be watching, so the
		// console names the new one and lets them decide. Publishing alone navigates
		// nowhere: the fear that started this was a cue yanking somebody off a page they
		// never acted from, and a toast with a link has no way to do that by accident.
		expect(goto).not.toHaveBeenCalled();

		toasts.items[0].action?.onselect();

		expect(goto).toHaveBeenCalledWith('/traces/trace-new');
	});

	it('keeps the recent-trace list on the trace detail route', () => {
		const { store, toasts } = harness(
			dashboardSummary({ recentInsights: [insightHighlight({ agentId: '' })] })
		);
		render(Dashboard, { store, toasts });

		expect(screen.getByRole('link', { name: 'All traces' })).toHaveAttribute('href', '/traces');
		expect(screen.getByRole('link', { name: 'Refactor the adapter seam' })).toHaveAttribute(
			'href',
			'/traces/trace-01a0bd6963faa14f'
		);
	});
});
