import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import System from './+page.svelte';
import { createSystemStore, type SystemStore } from '$lib/stores/system.svelte';
import { createAdapterCLIsStore } from '$lib/stores/adapters.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { adapterCLIs, systemProcess, systemView } from '../../tests/fixtures';
import type { AgentItem, AdapterCLIs, ChromeFrame, SystemView } from '$lib/api/types';

function agentItem(overrides: Partial<AgentItem> = {}): AgentItem {
	return {
		kind: 'afk',
		bee: 'scout',
		pid: 4242,
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'agent-1',
		startedAt: '2026-09-25T18:04:22Z',
		runDir: '/colony/.paseka/runs/trace-01a0bd6963faa14f/agent-1',
		...overrides
	};
}

/** A chrome stream carrying the live bee pids, with no stream of its own. */
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
	view: SystemView = systemView(),
	overrides: Partial<Parameters<typeof createSystemStore>[0]> = {}
): {
	store: SystemStore;
	status: ReturnType<typeof statusStore>;
	loadAdapters: ReturnType<typeof vi.fn>;
	adapters: ReturnType<typeof createAdapterCLIsStore>;
} {
	const store = createSystemStore({ loadSystem: async () => view, pollIntervalMs: 0, ...overrides });
	// The default answer is 2 of 4 found; a test that cares says otherwise.
	const loadAdapters = vi.fn(async (_refresh: boolean) => adapterCLIs());
	const adapters = createAdapterCLIsStore({ loadAdapters });
	return { store, status: statusStore(), loadAdapters, adapters };
}

/** The metric tile with this id, so a tile label is not confused with a table column. */
function tile(id: string): HTMLElement {
	const node = document.getElementById(id);
	if (!node) throw new Error(`tile ${id} not found`);
	return node;
}

/** Open the collapsible process block, which starts folded. */
async function openProcesses(): Promise<HTMLElement> {
	const toggle = screen.getByText('Processes', { selector: 'span' }).closest('summary');
	if (!toggle) throw new Error('process section summary not found');
	await userEvent.click(toggle);
	return within(toggle.parentElement as HTMLElement).getByLabelText('Processes');
}

/** The Agent CLIs block summary, which is both the fold control and the trigger. */
function agentCLIsToggle(): HTMLElement {
	const toggle = screen.getByText('Agent CLIs', { selector: 'span' }).closest('summary');
	if (!toggle) throw new Error('agent CLIs section summary not found');
	return toggle;
}

/** Open the folded Agent CLIs block and return its table region. */
async function openAgentCLIs(): Promise<HTMLElement> {
	const toggle = agentCLIsToggle();
	await userEvent.click(toggle);
	return within(toggle.parentElement as HTMLElement).getByLabelText('Agent CLIs');
}

/** Fold an open block again, which is the only way a lazy block is read twice. */
async function foldAgentCLIs(): Promise<void> {
	await userEvent.click(agentCLIsToggle());
}

describe('system route', () => {
	it('leads with the metrics an operator opens this page for', async () => {
		const { store, status } = harness();
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(tile('system-cpu')).toHaveTextContent('18%');
		expect(tile('system-memory')).toHaveTextContent('6.00 GiB / 16.0 GiB');
		// Available, load 5/15, and disk are the three the Host plaque cannot show.
		expect(tile('system-available')).toHaveTextContent('10.0 GiB');
		expect(tile('system-load')).toHaveTextContent('1.42 / 0.98 / 0.61');
		expect(tile('system-disk')).toHaveTextContent('40.0 GiB / 200 GiB');
	});

	it('drops a metric the server could not measure instead of padding it with a dash', async () => {
		// A network mount that refuses statfs must not blank the tab, and an `—`
		// tile would read as a fault the operator cannot act on.
		const { store, status } = harness(
			systemView({ diskUsedBytes: undefined, diskTotalBytes: undefined })
		);
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(screen.queryByText('Colony disk')).not.toBeInTheDocument();
		expect(tile('system-cpu')).toBeInTheDocument();
	});

	it('keeps the cpu tile and explains the gap when the first sample has no delta', async () => {
		const { store, status } = harness(systemView({ cpuPercent: undefined }));
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		// The tile stays, because a first-sample gap is expected rather than a fault,
		// and it carries the reason rather than a bare dash.
		expect(tile('system-cpu')).toHaveTextContent('—');
		expect(document.body.textContent).toContain('difference between two /proc/stat samples');
	});

	it('names the box and offers the values an operator pastes elsewhere', async () => {
		const { store, status } = harness();
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		const identity = screen.getByLabelText('Host identity');
		expect(within(identity).getByText('apiary')).toBeInTheDocument();
		expect(within(identity).getByText('6.11.0-21-generic')).toBeInTheDocument();
		expect(within(identity).getByText('linux / amd64')).toBeInTheDocument();
		expect(within(identity).getByText('7d 0h')).toBeInTheDocument();
		expect(within(identity).getByRole('button', { name: 'Copy hostname' })).toBeInTheDocument();
		expect(within(identity).getByRole('button', { name: 'Copy go' })).toBeInTheDocument();
	});

	it('folds the process table away and says how much is in it', async () => {
		const { store, status } = harness();
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(screen.getByText('3 busiest')).toBeInTheDocument();
		// A collapsed <details> still holds its children in the DOM, so the assertion
		// is on the open state rather than on presence.
		expect(document.getElementById('system-processes')).not.toHaveAttribute('open');
	});

	it('marks a live adapter in the table as a badge rather than a row class', async () => {
		const { store, status } = harness();
		store.stop();
		const withBee = statusStore([agentItem({ pid: 1187 })]);
		render(System, { store, status: withBee });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openProcesses();

		const beeRow = within(table).getByText('1187').closest('tr');
		expect(within(beeRow as HTMLElement).getByText('bee')).toBeInTheDocument();
		// An ordinary process keeps an empty cell rather than a dash or a false badge.
		const plainRow = within(table).getByText('909').closest('tr');
		expect(within(plainRow as HTMLElement).queryByText('bee')).not.toBeInTheDocument();
	});

	it('scales process memory to the process, not the machine', async () => {
		const { store, status } = harness(
			systemView({ processes: [systemProcess({ pid: 909, rssBytes: 3_145_728 })] })
		);
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openProcesses();

		// The GiB formatter would print 0.00 GiB here and make rows incomparable.
		expect(within(table).getByText('3.0 MiB')).toBeInTheDocument();
	});

	it('filters the table on a name, a command, or a pid', async () => {
		const { store, status } = harness();
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openProcesses();

		await userEvent.type(within(table).getByLabelText('Filter processes'), 'java');

		expect(within(table).getByText('1187')).toBeInTheDocument();
		expect(within(table).queryByText('909')).not.toBeInTheDocument();
	});

	it('says why the process list is missing instead of showing an empty table', async () => {
		const { store, status } = harness(systemView({ processes: undefined }));
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(screen.getByText('unavailable')).toBeInTheDocument();
		const table = await openProcesses();
		expect(within(table).getByText(/unavailable on this OS/)).toBeInTheDocument();
	});

	it('warns about a partial snapshot without discarding what did arrive', async () => {
		// The server answers 200 with an `error` string, so this is a warning about
		// missing rows, not a broken page.
		const { store, status } = harness(systemView({ error: 'loadavg: permission denied' }));
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(screen.getByRole('alert')).toHaveTextContent('Partial snapshot: loadavg: permission denied');
		expect(within(screen.getByLabelText('Host identity')).getByText('apiary')).toBeInTheDocument();
	});

	it('fails loudly and drops the skeletons when the read itself fails', async () => {
		const store = createSystemStore({
			loadSystem: async () => {
				throw new Error('connection refused');
			},
			pollIntervalMs: 0
		});
		render(System, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('connection refused'));

		expect(screen.queryByText('Memory')).not.toBeInTheDocument();
		expect(document.querySelector('[aria-busy="true"]')).toBeNull();
	});

	it('shows skeletons before the first payload', () => {
		const store = createSystemStore({ pollIntervalMs: 0 });
		render(System, { store, status: statusStore() });

		expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
		expect(screen.queryByText('Memory')).not.toBeInTheDocument();
	});

	it('does not claim the console is observe-only while offering a control', async () => {
		const { store, status } = harness();
		render(System, { store, status });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		// No kill, nice, or signal affordance anywhere on the page.
		expect(screen.queryByRole('button', { name: /kill|signal|nice/i })).not.toBeInTheDocument();
		expect(screen.getByText(/Observe-only/)).toBeInTheDocument();
	});
});

describe('agent CLIs block', () => {
	it('probes nothing on arrival and reads once when the block is opened', async () => {
		// The probe execs four external binaries and the server caches the answer,
		// so an operator who opens System should not pay for it until the block is
		// opened — and should not pay again for opening it twice.
		const { store, status, loadAdapters, adapters } = harness();
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		expect(loadAdapters).not.toHaveBeenCalled();
		expect(document.getElementById('system-agent-clis')).not.toHaveAttribute('open');

		await openAgentCLIs();
		await waitFor(() => expect(loadAdapters).toHaveBeenCalledTimes(1));

		// Fold and re-open: the payload is already here, so the request is not
		// repeated, which is what "one read per block" has to mean.
		await foldAgentCLIs();
		await openAgentCLIs();
		expect(loadAdapters).toHaveBeenCalledTimes(1);
	});

	it('says what it holds before a probe and how many it found after', async () => {
		const { store, status, adapters } = harness();
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		// The note is the only thing an operator has before clicking, so it states
		// the block's size and admits it knows nothing about this box yet.
		expect(screen.getByText('not probed, 4 adapters')).toBeInTheDocument();

		await openAgentCLIs();

		await waitFor(() => expect(screen.getByText('2 of 4 found')).toBeInTheDocument());
	});

	it('shows the three facts a row is about, and a missing CLI as a state', async () => {
		const { store, status, adapters } = harness();
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openAgentCLIs();

		const found = within(table).getByText('cursor').closest('tr') as HTMLElement;
		expect(within(found).getByText('found')).toBeInTheDocument();
		expect(within(found).getByText('/usr/local/bin/agent')).toBeInTheDocument();
		expect(within(found).getByText('0.48.4')).toBeInTheDocument();

		// Not installed is a verdict with a badge, an empty path, and an empty
		// version — not a failed row.
		const missing = within(table).getByText('claude').closest('tr') as HTMLElement;
		expect(within(missing).getByText('not found')).toBeInTheDocument();
		expect(within(missing).queryByText('/usr/local/bin/claude')).not.toBeInTheDocument();
	});

	it('hides the path below 768px rather than scrolling the table sideways', async () => {
		const { store, status, adapters } = harness();
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openAgentCLIs();

		// Five columns, so one is `secondary`: the path is the reference behind the
		// identity and the first thing to go.
		const path = within(table).getByRole('columnheader', { name: 'Path' });
		expect(path.className).toContain('hidden');
		expect(within(table).getByRole('columnheader', { name: 'Adapter' }).className).not.toContain(
			'hidden'
		);
	});

	it('re-probes on Refresh and shows the new run', async () => {
		const { store, status, loadAdapters, adapters } = harness();
		loadAdapters.mockImplementation(
			async (refresh: boolean) =>
				refresh
					? adapterCLIs({
							adapters: [
								{
									name: 'cursor',
									binary: 'agent',
									found: true,
									path: '/opt/agent',
									version: '0.49.0'
								}
							]
						})
					: adapterCLIs()
		);
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openAgentCLIs();
		await waitFor(() => expect(within(table).getByText('0.48.4')).toBeInTheDocument());

		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));

		// The refresh is what tells the server to drop its cache, so the request
		// carries that intent rather than being another plain read.
		await waitFor(() => expect(loadAdapters).toHaveBeenLastCalledWith(true));
		expect(await within(table).findByText('0.49.0')).toBeInTheDocument();
		expect(screen.getByText('1 of 1 found')).toBeInTheDocument();
	});

	it('says so when the probe answered with nothing', async () => {
		const { store, status } = harness();
		const adapters = createAdapterCLIsStore({
			loadAdapters: async () => adapterCLIs({ adapters: [] })
		});
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openAgentCLIs();

		expect(within(table).getByText(/the server probed none/)).toBeInTheDocument();
		expect(screen.getByText('0 of 0 found')).toBeInTheDocument();
	});

	it('keeps its filter out of the process table query', async () => {
		// Two tables on one route means two URL namespaces: without one, typing here
		// would overwrite the process filter an operator narrowed and shared.
		window.history.replaceState(null, '', '/next/system');
		const { store, status, adapters } = harness();
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		await openAgentCLIs();

		await userEvent.type(screen.getByLabelText('Filter agent CLIs'), 'opencode');

		await waitFor(() => expect(window.location.search).toBe('?agent-clis.q=opencode'));
	});

	it('skeletons the rows while the first probe is in flight', async () => {
		const { store, status } = harness();
		let release: (view: AdapterCLIs) => void = () => {};
		const adapters = createAdapterCLIsStore({
			loadAdapters: () =>
				new Promise<AdapterCLIs>((resolve) => {
					release = resolve;
				})
		});
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());
		const table = await openAgentCLIs();

		expect(table.querySelector('.skeleton')).not.toBeNull();
		expect(within(table).queryByText('0.48.4')).not.toBeInTheDocument();

		release(adapterCLIs());
		await waitFor(() => expect(within(table).getByText('0.48.4')).toBeInTheDocument());
	});
});

describe('agent CLIs failures', () => {
	it('reports a failed read and retries on the next open', async () => {
		const { store, status } = harness();
		const loadAdapters = vi
			.fn<() => Promise<AdapterCLIs>>()
			.mockRejectedValueOnce(new Error('connection refused'))
			.mockResolvedValueOnce(adapterCLIs());
		const adapters = createAdapterCLIsStore({ loadAdapters });
		render(System, { store, status, adapters });
		await waitFor(() => expect(screen.getByText('Memory')).toBeInTheDocument());

		await openAgentCLIs();
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('connection refused'));

		// Folded and re-opened, the block retries rather than staying a dead alert.
		await foldAgentCLIs();
		await openAgentCLIs();
		await waitFor(() => expect(loadAdapters).toHaveBeenCalledTimes(2));
		expect(await within(screen.getByLabelText('Agent CLIs')).findByText('0.48.4')).toBeInTheDocument();
	});
});
