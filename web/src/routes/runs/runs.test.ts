import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Runs from './+page.svelte';
import RunDetail from './[traceId]/[agentId]/RunDetail.svelte';
import { createRunStore, type RunStore } from '$lib/stores/run.svelte';
import { protocolEvent, runList, runSummary } from '../../tests/fixtures';
import type { ProtocolEvent, RunSummary } from '$lib/api/types';

type EventPage = { entries: ProtocolEvent[]; nextCursor: number };

function harness(
	runs: RunSummary[] = runList(),
	pages: EventPage[] = [{ entries: [protocolEvent()], nextCursor: 1 }]
) {
	const listRuns = vi.fn(async () => runs);
	const listRunEvents = vi.fn(async () => pages.shift() ?? { entries: [], nextCursor: 0 });
	const getRun = vi.fn(async (traceId: string, agentId: string) => runSummary({ traceId, agentId }));
	const store = createRunStore({ listRuns, listRunEvents, getRun, pollIntervalMs: 0 });
	return { store, listRuns, listRunEvents, getRun };
}

async function openRun(traceId: string, agentId: string) {
	const h = harness();
	render(RunDetail, { store: h.store, traceId, agentId });
	await waitFor(() => expect(h.listRunEvents).toHaveBeenCalled());
	return h;
}

describe('runs list', () => {
	it('lists the recent runs with the columns an operator scans', async () => {
		const { store } = harness();
		render(Runs, { store });
		await waitFor(() => expect(screen.getByLabelText('Runs')).toBeInTheDocument());

		const table = screen.getByLabelText('Runs');
		for (const label of ['Started', 'Bee', 'State', 'Trail', 'Task', 'Run']) {
			expect(within(table).getByRole('columnheader', { name: label })).toBeInTheDocument();
		}
		// The list is compact: the adapter is searchable rather than a column, because
		// which adapter ran is a question the detail page answers.
		expect(within(table).getAllByText('builder')).not.toHaveLength(0);
		expect(within(table).queryByText('claude-code')).not.toBeInTheDocument();
	});

	it('badges the state, which is what decides whether a run is read', async () => {
		const { store } = harness();
		render(Runs, { store });
		await waitFor(() => expect(screen.getByLabelText('Runs')).toBeInTheDocument());

		const table = screen.getByLabelText('Runs');
		expect(within(table).getByText('running')).toBeInTheDocument();
		expect(within(table).getByText('failed')).toBeInTheDocument();
		// The fixture holds two completed runs, so the word appears twice.
		expect(within(table).getAllByText('completed')).toHaveLength(2);
	});

	it('links a run to its detail and its trail to the trail page', async () => {
		const { store } = harness();
		render(Runs, { store });
		await waitFor(() => expect(screen.getByLabelText('Runs')).toBeInTheDocument());

		const table = screen.getByLabelText('Runs');
		const runLink = within(table).getByRole('link', { name: 'run-01' });
		expect(runLink).toHaveAttribute(
			'href',
			'/next/runs/trace-01a0bd6963faa14f/run-01'
		);
		expect(within(table).getAllByRole('link', { name: 'trace-01a0bd6963faa14f' })[0]).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
	});

	it('filters on a hidden term, the adapter being the one that is not a column', async () => {
		const { store } = harness();
		render(Runs, { store });
		await waitFor(() => expect(screen.getByLabelText('Runs')).toBeInTheDocument());

		await userEvent.type(screen.getByLabelText('Filter runs'), 'claude-code');

		const table = screen.getByLabelText('Runs');
		expect(within(table).getByText('run-01')).toBeInTheDocument();
		expect(within(table).queryByText('run-02')).not.toBeInTheDocument();
	});

	it('says no runs rather than showing an empty table', async () => {
		const { store } = harness([]);
		render(Runs, { store });
		await waitFor(() => expect(screen.getByText('No runs recorded yet.')).toBeInTheDocument());
	});

	it('alerts and drops the skeletons when the read fails', async () => {
		const store = createRunStore({
			listRuns: async () => {
				throw new Error('colony root unreadable');
			},
			pollIntervalMs: 0
		});
		render(Runs, { store });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('colony root unreadable'));

		expect(document.querySelector('[aria-busy="true"]')).toBeNull();
	});
});

describe('run detail', () => {
	it('names the run and what it was asked to do', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');

		expect(screen.getByRole('heading', { name: 'run-01' })).toBeInTheDocument();
		const identity = screen.getByLabelText('Run identity');
		expect(within(identity).getByText('builder')).toBeInTheDocument();
		expect(within(identity).getByText('claude-code')).toBeInTheDocument();
		expect(within(identity).getByText('debugging')).toBeInTheDocument();
	});

	it('links the run back to its trail', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');

		expect(screen.getByLabelText('Run identity')).toHaveTextContent('trace-01a0bd6963faa14f');
		expect(screen.getAllByRole('link', { name: 'trace-01a0bd6963faa14f' })[0]).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
	});

	it('shows the token spend for a run that reported it', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');
		expect(screen.getByLabelText('Token spend')).toBeInTheDocument();
	});

	it('leaves the token spend out for a run that reported none', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-02');
		expect(screen.queryByLabelText('Token spend')).not.toBeInTheDocument();
	});

	it('keeps the adapter summary as literal text, terminal escapes and all', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-02');

		await userEvent.click(screen.getByText('Summary'));

		// A run summary is whatever the adapter wrote, so it is never treated as
		// markup or as prose to reflow.
		expect(screen.getByText('Wired the export format flag.')).toBeInTheDocument();
	});

	it('shows the task body folded, because it is long and rarely the first question', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');

		const section = document.getElementById('run-body');
		expect(section).not.toHaveAttribute('open');
		expect(screen.getByText('what the adapter was handed')).toBeInTheDocument();
	});
});

describe('formatted run events', () => {
	it('renders each recorded event as a feed row, not a raw pre', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');

		const feed = screen.getByLabelText('Run events');
		// The legacy drew `[TYPE #seq] {json}`; the projection is the readable row the
		// Dashboard and the Timeline already use.
		expect(within(feed).getByText('Committed the prune cleanup command.')).toBeInTheDocument();
		expect(within(feed).getByText(/INSIGHT · run\.summary/)).toBeInTheDocument();
		// The raw envelope is present but folded: a collapsed <details> keeps its
		// children in the DOM, so the assertion is on the open state.
		const raw = within(feed).getByText('Raw event #1').closest('details') as HTMLElement;
		expect(raw).not.toHaveAttribute('open');
	});

	it('falls back to the payload fields for an event with no summary', async () => {
		const h = harness(runList(), [
			{ entries: [protocolEvent({ payload: { kind: 'trace.title', title: 'Add prune cleanup' } })], nextCursor: 1 }
		]);
		render(RunDetail, { store: h.store, traceId: 'trace-01a0bd6963faa14f', agentId: 'run-01' });
		await waitFor(() => expect(screen.getByLabelText('Run events')).toBeInTheDocument());

		expect(within(screen.getByLabelText('Run events')).getByText('title=Add prune cleanup')).toBeInTheDocument();
	});

	it('reveals the raw envelope per row without a second request', async () => {
		const h = await openRun('trace-01a0bd6963faa14f', 'run-01');
		expect(h.listRunEvents).toHaveBeenCalledTimes(1);

		await userEvent.click(screen.getByText('Raw event #1'));

		expect(screen.getByText(/"protocolVersion": "1"/)).toBeInTheDocument();
		expect(h.listRunEvents).toHaveBeenCalledTimes(1);
	});

	it('says a run recorded nothing rather than showing an empty list', async () => {
		const h = harness(runList(), [{ entries: [], nextCursor: 0 }]);
		render(RunDetail, { store: h.store, traceId: 'trace-01a0bd6963faa14f', agentId: 'run-03' });
		await waitFor(() => expect(screen.getByText(/recorded no events/)).toBeInTheDocument());

		expect(screen.queryByLabelText('Run events')).not.toBeInTheDocument();
	});

	it('offers no paging control, because the first read is the whole log', async () => {
		const h = harness(runList(), [
			{ entries: [protocolEvent({ seq: 1 }), protocolEvent({ seq: 2 })], nextCursor: 2 }
		]);
		render(RunDetail, { store: h.store, traceId: 'trace-01a0bd6963faa14f', agentId: 'run-01' });
		await waitFor(() => expect(screen.getByLabelText('Run events')).toBeInTheDocument());

		// `nextCursor` is an index, not a promise of more, so a button here could only
		// ever confirm that there is nothing left.
		expect(screen.queryByRole('button', { name: /Load earlier events/ })).not.toBeInTheDocument();
		expect(h.listRunEvents).toHaveBeenCalledTimes(1);
	});

	it('reports an events failure without taking the run down with it', async () => {
		const store = createRunStore({
			listRuns: async () => runList(),
			listRunEvents: async () => {
				throw new Error('events.ndjson: permission denied');
			},
			pollIntervalMs: 0
		});
		render(RunDetail, { store, traceId: 'trace-01a0bd6963faa14f', agentId: 'run-01' });
		await waitFor(() =>
			expect(screen.getByRole('alert')).toHaveTextContent('events.ndjson: permission denied')
		);

		// The identity block is still there: a missing event log is not a missing run.
		expect(screen.getByLabelText('Run identity')).toBeInTheDocument();
	});
});

describe('previous and next run in the same trail', () => {
	it('steps by start time and says where it sits', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-02');

		expect(screen.getByText('2 of 3 in this trail')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Previous run in this trail' })).toHaveAttribute(
			'href',
			'/next/runs/trace-01a0bd6963faa14f/run-01'
		);
		expect(screen.getByRole('link', { name: 'Next run in this trail' })).toHaveAttribute(
			'href',
			'/next/runs/trace-01a0bd6963faa14f/run-03'
		);
	});

	it('is a real link, so a run in a trail can be shared and Back works', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-02');

		// A button that swapped the store would leave the address bar pointing at the
		// previous run.
		const link = screen.getByRole('link', { name: 'Next run in this trail' });
		expect(link.tagName).toBe('A');
	});

	it('disables the step at the newest run of a trail', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-03');

		expect(screen.getByRole('button', { name: 'Next run in this trail' })).toBeDisabled();
		expect(screen.getByRole('link', { name: 'Previous run in this trail' })).toBeInTheDocument();
	});

	it('disables the step at the oldest run of a trail', async () => {
		await openRun('trace-01a0bd6963faa14f', 'run-01');

		expect(screen.getByRole('button', { name: 'Previous run in this trail' })).toBeDisabled();
		expect(screen.getByRole('link', { name: 'Next run in this trail' })).toBeInTheDocument();
	});

	it('has no step at all for a trail with one run', async () => {
		await openRun('trace-01a09966fdbe6771', 'run-09');

		expect(screen.getByRole('button', { name: 'Previous run in this trail' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Next run in this trail' })).toBeDisabled();
		// One run is not "1 of 1"; the position line would be noise.
		expect(screen.queryByText(/in this trail/)).not.toBeInTheDocument();
	});

	it('does not offer another trail run as a sibling', async () => {
		await openRun('trace-01a09966fdbe6771', 'run-09');

		// run-09 is alone on its trail even though the list holds four runs.
		expect(screen.getByRole('button', { name: 'Previous run in this trail' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Next run in this trail' })).toBeDisabled();
	});

	it('says a run outside the recent window has no known position rather than guessing', async () => {
		await openRun('trace-older', 'run-99');

		expect(screen.queryByText(/in this trail/)).not.toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Previous run in this trail' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Next run in this trail' })).toBeDisabled();
	});
});

describe('run detail without a run', () => {
	it('says not found rather than rendering an empty shell', async () => {
		const store = createRunStore({
			listRuns: async () => [],
			getRun: async () => {
				throw new Error('run not found');
			},
			listRunEvents: async () => ({ entries: [], nextCursor: 0 }),
			pollIntervalMs: 0
		});
		render(RunDetail, { store, traceId: 'trace-nope', agentId: 'run-nope' });
		await waitFor(() => expect(screen.getByText('Run not found')).toBeInTheDocument());

		expect(screen.getByText(/older than the recent window/)).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'All runs' })).toHaveAttribute('href', '/next/runs');
	});
});
