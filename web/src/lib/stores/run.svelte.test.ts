import { describe, expect, it, vi } from 'vitest';
import { createRunStore } from './run.svelte';
import { protocolEvent, runList, runSummary } from '../../tests/fixtures';
import type { ProtocolEvent, RunSummary } from '$lib/api/types';

type EventPage = { entries: ProtocolEvent[]; nextCursor: number };

function harness(
	runs: RunSummary[] = runList(),
	pages: EventPage[] = [{ entries: [protocolEvent()], nextCursor: 1 }]
) {
	const listRuns = vi.fn(async () => runs);
	const listRunEvents = vi.fn(async () => pages.shift() ?? { entries: [], nextCursor: 0 });
	// A deep-linked run the recent window no longer holds, so the detail has to come
	// from the detail endpoint with the ids that were asked for.
	const getRun = vi.fn(async (traceId: string, agentId: string) =>
		runSummary({ traceId, agentId })
	);
	const store = createRunStore({ listRuns, listRunEvents, getRun, pollIntervalMs: 0 });
	return { store, listRuns, listRunEvents, getRun };
}

describe('createRunStore', () => {
	it('reads the recent runs and never polls them twice on mount', async () => {
		const { store, listRuns } = harness();
		expect(store.showSkeletons).toBe(true);

		await store.start();
		await store.start();

		expect(listRuns).toHaveBeenCalledTimes(1);
		expect(store.runs).toHaveLength(4);
		expect(store.showSkeletons).toBe(false);
	});

	it('keeps the last list on screen through a later failure', async () => {
		const listRuns = vi
			.fn<() => Promise<RunSummary[]>>()
			.mockResolvedValueOnce(runList())
			.mockRejectedValueOnce(new Error('colony root unreadable'));
		const store = createRunStore({ listRuns, pollIntervalMs: 0 });
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('colony root unreadable');
		expect(store.runs).toHaveLength(4);
	});

	it('polls, unlike the feed and the topology, because a run state moves', async () => {
		vi.useFakeTimers();
		try {
			const listRuns = vi.fn(async () => runList());
			const store = createRunStore({
				listRuns,
				listRunEvents: async () => ({ entries: [], nextCursor: 0 }),
				pollIntervalMs: 1000
			});

			store.start();
			await vi.advanceTimersByTimeAsync(1000);
			expect(listRuns).toHaveBeenCalledTimes(2);

			store.stop();
			await vi.advanceTimersByTimeAsync(5000);
			expect(listRuns).toHaveBeenCalledTimes(2);
		} finally {
			vi.useRealTimers();
		}
	});

	it('finds the selected run in the list without a second request', async () => {
		const { store, getRun } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-02');

		expect(store.current?.agentId).toBe('run-02');
		// The recent list already carries the whole run view, so a run inside the
		// window costs no detail request.
		expect(getRun).not.toHaveBeenCalled();
	});

	it('fetches a run the recent window no longer holds, so a deep link still works', async () => {
		const { store, getRun } = harness();
		await store.start();

		await store.select('trace-older', 'run-99');

		expect(getRun).toHaveBeenCalledWith('trace-older', 'run-99');
		expect(store.current?.agentId).toBe('run-99');
	});

	it('reads a selected run events once, keyed by both ids', async () => {
		const { store, listRunEvents } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-01');

		expect(listRunEvents).toHaveBeenCalledWith('trace-01a0bd6963faa14f', 'run-01');
		expect(store.events).toHaveLength(1);
	});

	it('replaces the events when a different run is selected, never appending across runs', async () => {
		const { store } = harness(runList(), [
			{ entries: [protocolEvent({ seq: 1 })], nextCursor: 1 },
			{ entries: [protocolEvent({ seq: 9, type: 'VERIFICATION' })], nextCursor: 1 }
		]);
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-01');
		await store.select('trace-01a0bd6963faa14f', 'run-02');

		expect(store.events.map((event) => event.seq)).toEqual([9]);
	});

	it('reads the whole log in one page, because the endpoint caps nothing', async () => {
		// `ReadEventsAfter` returns every event from its cursor, so `nextCursor` is an
		// index rather than a promise of more, and a paging control could only ever
		// come back empty.
		const { store, listRunEvents } = harness(runList(), [
			{ entries: [protocolEvent({ seq: 1 }), protocolEvent({ seq: 2 })], nextCursor: 2 }
		]);
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-01');

		expect(listRunEvents).toHaveBeenCalledTimes(1);
		expect(listRunEvents).toHaveBeenCalledWith('trace-01a0bd6963faa14f', 'run-01');
		expect(store.events.map((event) => event.seq)).toEqual([1, 2]);
	});

	it('reports an events failure apart from the run itself, which is still readable', async () => {
		const store = createRunStore({
			listRuns: async () => runList(),
			listRunEvents: async () => {
				throw new Error('events.ndjson: permission denied');
			},
			pollIntervalMs: 0
		});
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-01');

		expect(store.eventsError).toBe('events.ndjson: permission denied');
		expect(store.current?.agentId).toBe('run-01');
	});
});

describe('run position in its trail', () => {
	it('steps by start time, so a sibling is the run before or after this one', async () => {
		const { store } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-02');

		expect(store.position).toMatchObject({ index: 1, total: 3 });
		// Newest first, so "previous" is the older run and "next" the newer one.
		expect(store.position.previous?.agentId).toBe('run-01');
		expect(store.position.next?.agentId).toBe('run-03');
	});

	it('leaves a run with no older sibling nothing to step back to', async () => {
		const { store } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-01');

		expect(store.position.previous).toBeNull();
		expect(store.position.next?.agentId).toBe('run-02');
	});

	it('leaves a run with no newer sibling nothing to step forward to', async () => {
		const { store } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', 'run-03');

		expect(store.position.next).toBeNull();
		expect(store.position.previous?.agentId).toBe('run-02');
	});

	it('does not count another trail runs as siblings', async () => {
		const { store } = harness();
		await store.start();

		await store.select('trace-01a09966fdbe6771', 'run-09');

		// run-09 is alone on its trail even though the list holds four runs.
		expect(store.position).toMatchObject({ index: 0, total: 1, previous: null, next: null });
	});

	it('reports nothing before a run is selected', async () => {
		const { store } = harness();
		await store.start();

		expect(store.selected).toBe(false);
		expect(store.position.index).toBe(-1);
	});

	it('reports a detached run outside the recent window as unpositioned rather than wrong', async () => {
		const { store } = harness();
		await store.start();

		await store.select('trace-older', 'run-99');

		expect(store.current?.agentId).toBe('run-99');
		// Its siblings are older than the recent list, so the position is unknown
		// rather than a confident 1-of-1.
		expect(store.position.index).toBe(-1);
	});
});
