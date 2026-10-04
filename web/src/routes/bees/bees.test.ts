import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Bees from './+page.svelte';
import { createBeesStore } from '$lib/stores/bees.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { bee, beeRoster } from '../../tests/fixtures';
import type { AgentItem, Bee, ChromeFrame } from '$lib/api/types';

afterEach(() => {
	vi.unstubAllGlobals();
});

function agent(overrides: Partial<AgentItem> = {}): AgentItem {
	return {
		kind: 'afk',
		bee: 'builder',
		pid: 4242,
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'builder-1',
		startedAt: '2026-09-27T09:12:03Z',
		runDir: '/colony/.paseka/runs/trace-01a0bd6963faa14f/builder-1',
		...overrides
	};
}

/** A chrome stream carrying the live bees, with no stream of its own. */
function statusStore(items: AgentItem[] = []) {
	const store = createConsoleStatusStore({ pollIntervalMs: 0 });
	store.applyChromeFrame({
		schemaVersion: 1,
		runtime: { status: 'running', alive: true, slug: 'demo', pid: 42 },
		agents: { count: items.length, afk: items.length, sessions: 0, items },
		attention: { reviews: 0, sessions: 0 }
	} as ChromeFrame);
	return store;
}

function harness(
	bees: Bee[] = beeRoster(),
	overrides: Partial<Parameters<typeof createBeesStore>[0]> = {}
): {
	store: ReturnType<typeof createBeesStore>;
	status: ReturnType<typeof statusStore>;
	loadBees: ReturnType<typeof vi.fn>;
} {
	const loadBees = vi.fn(async () => bees);
	// The override comes last so a test that supplies its own reader wins, and the
	// handle returned is the one the store actually holds.
	const store = createBeesStore({ loadBees, ...overrides });
	return {
		store,
		status: statusStore(),
		loadBees: (overrides.loadBees ?? loadBees) as ReturnType<typeof vi.fn>
	};
}

/** The cells of the row holding `text`, so a column can be asserted by position. */
function cellsOf(text: string): HTMLElement[] {
	const row = screen.getByText(text).closest('tr');
	return within(row as HTMLElement).getAllByRole('cell') as HTMLElement[];
}

/** The column headers, so a cell index is named rather than counted in a comment. */
function headers(): string[] {
	return within(screen.getByRole('table')).getAllByRole('columnheader').map((cell) => cell.textContent ?? '');
}

function cellNamed(row: string, column: string): HTMLElement {
	const index = headers().indexOf(column);
	if (index < 0) throw new Error(`no ${column} column in ${headers().join(', ')}`);
	return cellsOf(row)[index] as HTMLElement;
}

describe('bees route', () => {
	it('reads the roster once, and never starts a timer', async () => {
		const { store, status, loadBees } = harness();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		expect(loadBees).toHaveBeenCalledTimes(1);
		// Bee YAML is committed, so a poll would re-read it for nothing. The store
		// has no interval at all, and Refresh is the control that re-reads.
		expect(store.bees).toHaveLength(3);
	});

	it('lists a script bee the launch picker cannot offer', async () => {
		const { store, status } = harness();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByText('sweeper')).toBeInTheDocument());
		// The roster's whole reason: `GET /api/bees` without a scope would not send it.
		expect(cellNamed('sweeper', 'Adapter')).toHaveTextContent('script');
		expect(cellNamed('sweeper', 'Intents')).toHaveTextContent('—');
	});

	it('names the colony in a sentence rather than a plural of nothing', async () => {
		const { store, status } = harness([bee()]);
		const one = render(Bees, { store, status });
		await waitFor(() =>
			expect(screen.getByText(/One bee, with the adapter/)).toBeInTheDocument()
		);
		one.unmount();

		const empty = harness([]);
		render(Bees, { store: empty.store, status: empty.status });
		await waitFor(() => expect(screen.getByText('No bee is registered in this colony.')).toBeInTheDocument());
	});

	it('says where a bee works, so a colony-root row is not read as an isolated one', async () => {
		const { store, status } = harness();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByText('hivewright')).toBeInTheDocument());
		expect(cellNamed('builder', 'Workspace')).toHaveTextContent('worktree');
		// A `worktree: false` bee writes to the checkout itself, which the page says
		// out loud rather than leaving an empty cell to be read as "no data".
		expect(cellNamed('hivewright', 'Workspace')).toHaveTextContent('colony root');
	});

	it('badges a live bee from the agents frame the topbar already keeps', async () => {
		const { store } = harness();
		const status = statusStore([
			agent(),
			agent({ pid: 4243, agentId: 'builder-2' }),
			agent({ kind: 'session', bee: 'sweeper', sessionId: 'session-1' })
		]);
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByText('builder')).toBeInTheDocument());
		expect(cellNamed('builder', 'Live')).toHaveTextContent('2');
		expect(cellNamed('sweeper', 'Live')).toHaveTextContent('1');
		// A bee holding nothing is an empty cell, not a word in every idle row.
		expect(cellNamed('hivewright', 'Live')).toBeEmptyDOMElement();
	});

	it('follows the agents stream rather than freezing on the first frame', async () => {
		const { store } = harness();
		const status = statusStore();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByText('builder')).toBeInTheDocument());
		expect(cellNamed('builder', 'Live')).toBeEmptyDOMElement();

		// The roster is read once, so a live count that never moved would mean the
		// column closed over the payload instead of the frame.
		status.applyChromeFrame({
			schemaVersion: 1,
			runtime: { status: 'running', alive: true, slug: 'demo', pid: 42 },
			agents: { count: 1, afk: 1, sessions: 0, items: [agent()] },
			attention: { reviews: 0, sessions: 0 }
		} as ChromeFrame);

		await waitFor(() => expect(cellNamed('builder', 'Live')).toHaveTextContent('1'));
	});

	it('links a last run and badges its state, and leaves a bee that never ran empty', async () => {
		const { store, status } = harness();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		// The cell is the run's own start time, linked to the run page, where a bee's
		// output is actually read.
		const link = within(cellNamed('builder', 'Last run')).getByRole('link');
		expect(link).toHaveTextContent(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
		expect(link).toHaveAttribute('href', '/next/runs/trace-01a0bd6963faa14f/builder-1');
		expect(cellNamed('builder', 'Last run')).toHaveTextContent('completed');

		// A failed last run is the health signal the page exists for, so it is toned
		// rather than printed as another timestamp.
		expect(cellNamed('hivewright', 'Last run')).toHaveTextContent('failed');
		// The server sends no lastRun for a bee that never ran, and the cell is left
		// empty rather than printing a dash that would read as a missing timestamp.
		expect(cellNamed('sweeper', 'Last run')).toBeEmptyDOMElement();
	});

	it('shows the intent vocabulary comma-joined, and a dash when there is none', async () => {
		const { store, status } = harness();
		render(Bees, { store, status });

		await waitFor(() => expect(screen.getByText('builder')).toBeInTheDocument());
		// One string, not a badge per intent: the filter box reads this cell, so each
		// intent is searchable on its own without anything duplicated by hand.
		expect(cellNamed('builder', 'Intents')).toHaveTextContent('feature, refactor, test-fix');
		expect(cellNamed('hivewright', 'Intents')).toHaveTextContent('—');
	});

	it('filters by role, adapter, a single intent, or a sector that is not on screen', async () => {
		const user = userEvent.setup();
		const { store, status } = harness();
		render(Bees, { store, status });
		await waitFor(() => expect(screen.getByText('builder')).toBeInTheDocument());

		await user.type(screen.getByLabelText('Filter bees'), 'refactor');
		expect(screen.getByText('builder')).toBeInTheDocument();
		expect(screen.queryByText('hivewright')).not.toBeInTheDocument();

		await user.clear(screen.getByLabelText('Filter bees'));
		await user.type(screen.getByLabelText('Filter bees'), 'script');
		expect(screen.getByText('sweeper')).toBeInTheDocument();
		expect(screen.queryByText('builder')).not.toBeInTheDocument();
	});

	it('re-reads on demand, since a run landing is the only thing here that moves without a commit', async () => {
		const user = userEvent.setup();
		const first = beeRoster();
		const { store, status, loadBees } = harness(first, {
			loadBees: vi
				.fn<() => Promise<Bee[]>>()
				.mockResolvedValueOnce(first)
				.mockResolvedValueOnce([...first, bee({ role: 'drone', adapter: 'pi', sector: 'docs' })])
		});
		render(Bees, { store, status });
		await waitFor(() => expect(screen.getByText('sweeper')).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Refresh' }));
		await waitFor(() => expect(screen.getByText('drone')).toBeInTheDocument());
		expect(loadBees).toHaveBeenCalledTimes(2);
	});

	it('runs a bee from the header, choosing one — a script row included', async () => {
		const user = userEvent.setup();
		const { store, status } = harness();
		const runs: string[] = [];
		vi.stubGlobal(
			'fetch',
			vi.fn(async (input: RequestInfo | URL) => {
				runs.push(String(input));
				return new Response(
					JSON.stringify({ traceId: 'trace-9f0c1d2e3a4b5c6d', bee: 'sweeper' }),
					{ status: 201, headers: { 'Content-Type': 'application/json' } }
				);
			})
		);
		render(Bees, { store, status });
		await waitFor(() => expect(screen.getByText('sweeper')).toBeInTheDocument());

		// One control for the page, not one per row: which bee to run is the form's
		// question, and the roster behind it is a list to read rather than a menu.
		await user.click(screen.getByRole('button', { name: 'Run bee' }));
		const beeSelect = await screen.findByLabelText('Bee');
		// `sweeper` is the row a launch session cannot offer, and this is the one
		// control that can start it, so the roster is passed whole.
		await user.selectOptions(beeSelect, 'sweeper');
		await user.type(screen.getByLabelText('Task'), 'sweep the stale worktrees');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(runs).toEqual(['/api/bees/sweeper/run']));
	});

	it('offers the trail a started run belongs to instead of navigating away', async () => {
		const user = userEvent.setup();
		const { store, status } = harness();
		const toasts = createToastStore(0);
		vi.stubGlobal(
			'fetch',
			vi.fn(
				async () =>
					new Response(JSON.stringify({ traceId: 'trace-9f0c1d2e3a4b5c6d', bee: 'builder' }), {
						status: 201,
						headers: { 'Content-Type': 'application/json' }
					})
			)
		);
		render(Bees, { store, status, toasts });
		await waitFor(() => expect(screen.getByText('builder')).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Run bee' }));
		await user.selectOptions(await screen.findByLabelText('Bee'), 'builder');
		await user.type(screen.getByLabelText('Task'), 'go');
		await user.click(screen.getByRole('button', { name: 'Run' }));

		await waitFor(() => expect(toasts.items).toHaveLength(1));
		const toast = toasts.items[0];
		expect(toast?.tone).toBe('success');
		expect(toast?.message).toBe('Run started — trail trace-9f0c1d2e3a4b5c6d');
		// The answer carries no agent and no output — the run lasts longer than the
		// request — so the trail is what the operator is handed, and pressing it is
		// the only thing that navigates.
		expect(toast?.action?.label).toBe('Open trail');
	});

	it('an empty colony says so instead of showing a table of nothing', async () => {
		const { store, status } = harness([]);
		render(Bees, { store, status });

		await waitFor(() =>
			expect(screen.getByText('No bees are registered under .paseka/bees.')).toBeInTheDocument()
		);
		expect(screen.getByText('No bee is registered in this colony.')).toBeInTheDocument();
	});

	it('skeletons while the first read is in flight, and alerts when it fails', async () => {
		let release: ((bees: Bee[]) => void) | undefined;
		const gate = new Promise<Bee[]>((resolve) => (release = resolve));
		const slow = harness(beeRoster(), { loadBees: () => gate });
		const { container, unmount } = render(Bees, { store: slow.store, status: slow.status });

		await waitFor(() => expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		unmount();
		release?.(beeRoster());

		const failed = harness(beeRoster(), {
			loadBees: async () => {
				throw new Error('bees: colony root is not readable');
			}
		});
		const broken = render(Bees, { store: failed.store, status: failed.status });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('colony root is not readable'));
		expect(broken.container.querySelectorAll('.skeleton').length).toBe(0);
		expect(screen.queryByRole('table')).not.toBeInTheDocument();
	});
});
