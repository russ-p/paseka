import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './+page.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { dashboardSummary, insightHighlight, runSummary, traceSummary } from '../../tests/fixtures';
import type { DashboardSummary } from '$lib/api/types';

function harness(summary: DashboardSummary = dashboardSummary()) {
	const store = createConsoleStatusStore({ pollIntervalMs: 0 });
	store.applyChromeFrame({ schemaVersion: 1, runtime: { status: 'running', alive: true, slug: 'paseka' } });
	store.applyDashboard(summary);
	return { store, toasts: createToastStore(0) };
}

afterEach(() => {
	vi.unstubAllGlobals();
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
		expect(screen.getByText('Active worktrees').closest('.stat')).toHaveTextContent('3');
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

	it('keeps the recent-trace list on the trace detail route', () => {
		const { store, toasts } = harness(
			dashboardSummary({ recentInsights: [insightHighlight({ agentId: '' })] })
		);
		render(Dashboard, { store, toasts });

		expect(screen.getByRole('link', { name: 'All traces' })).toHaveAttribute('href', '/next/traces');
		expect(screen.getByRole('link', { name: 'Refactor the adapter seam' })).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
	});
});
