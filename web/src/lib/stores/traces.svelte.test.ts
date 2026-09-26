import { describe, expect, it, vi } from 'vitest';
import { createTracesStore } from './traces.svelte';
import { traceSummary } from '../../tests/fixtures';
import type { TraceSummary } from '$lib/api/types';

function page(ids: string[], at: string): TraceSummary[] {
	return ids.map((traceId) => traceSummary({ traceId, lastActivityAt: at }));
}

describe('createTracesStore', () => {
	it('loads the first page and stops offering more once the server runs dry', async () => {
		const loadTraces = vi.fn(async () => page(['a', 'b'], '2026-09-25T18:04:22Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });

		expect(store.showSkeletons).toBe(true);
		await store.refresh();

		expect(store.traces.map((trace) => trace.traceId)).toEqual(['a', 'b']);
		expect(store.showSkeletons).toBe(false);
		expect(store.hasMore).toBe(true);
		await store.loadMore();
		expect(loadTraces).toHaveBeenCalledTimes(2);
	});

	it('appends the next page from the cursor of the last row it holds', async () => {
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a', 'b'], '2026-09-25T18:04:22Z'))
			.mockResolvedValueOnce(page(['c'], '2026-09-24T10:00:00Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();

		await store.loadMore();

		expect(loadTraces).toHaveBeenLastCalledWith({
			limit: 2,
			before: '2026-09-25T18:04:22Z|b'
		});
		expect(store.traces.map((trace) => trace.traceId)).toEqual(['a', 'b', 'c']);
		expect(store.hasMore).toBe(false);
	});

	it('never asks for another page when the trail history is exhausted', async () => {
		const loadTraces = vi.fn(async () => page(['a'], '2026-09-25T18:04:22Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();
		expect(store.hasMore).toBe(false);

		await store.loadMore();
		expect(loadTraces).toHaveBeenCalledTimes(1);
	});

	it('keeps the older pages an operator pulled in when a poll returns', async () => {
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a', 'b'], '2026-09-25T18:04:22Z'))
			.mockResolvedValueOnce(page(['c'], '2026-09-24T10:00:00Z'))
			.mockResolvedValueOnce(page(['z', 'a', 'b'], '2026-09-26T09:00:00Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();
		await store.loadMore();
		expect(store.traces).toHaveLength(3);

		await store.refresh();

		expect(store.traces.map((trace) => trace.traceId)).toEqual(['z', 'a', 'b', 'c']);
	});

	it('reports a failure without discarding the rows already on screen', async () => {
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a'], '2026-09-25T18:04:22Z'))
			.mockRejectedValueOnce(new Error('nats url not configured'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();

		await store.refresh();

		expect(store.lastError).toBe('nats url not configured');
		expect(store.traces).toHaveLength(1);
	});

	it('refuses to start a second page load while one is in flight', async () => {
		let release: (() => void) | undefined;
		const gate = new Promise<void>((resolve) => (release = resolve));
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a', 'b'], '2026-09-25T18:04:22Z'))
			.mockImplementationOnce(async () => {
				await gate;
				return page(['c'], '2026-09-24T10:00:00Z');
			});
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();

		const first = store.loadMore();
		const second = store.loadMore();
		expect(store.loadingMore).toBe(true);
		release?.();
		await Promise.all([first, second]);

		expect(loadTraces).toHaveBeenCalledTimes(2);
		expect(store.loadingMore).toBe(false);
	});
});
