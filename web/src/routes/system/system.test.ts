import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import System from './+page.svelte';
import { createSystemStore, type SystemStore } from '$lib/stores/system.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { systemProcess, systemView } from '../../tests/fixtures';
import type { AgentItem, ChromeFrame, SystemView } from '$lib/api/types';

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
): { store: SystemStore; status: ReturnType<typeof statusStore> } {
	const store = createSystemStore({ loadSystem: async () => view, pollIntervalMs: 0, ...overrides });
	return { store, status: statusStore() };
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
