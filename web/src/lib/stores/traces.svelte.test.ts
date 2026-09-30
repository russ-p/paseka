import { describe, expect, it, vi } from 'vitest';
import { createTracesStore } from './traces.svelte';
import { traceSummary } from '../../tests/fixtures';
import type { TraceSummary } from '$lib/api/types';

function page(ids: string[], at: string): TraceSummary[] {
	return ids.map((traceId) => traceSummary({ traceId, lastActivityAt: at }));
}

/**
 * A server that hands out `trace-<n>` pages from a fixed history, honouring the
 * cursor the way `hiveview.ScanTracesAfter` does: strictly *after* the row the
 * cursor names, and one row past whatever was asked for.
 */
function pagedHistory(total: number) {
	return async ({ limit = 3, before }: { limit?: number; before?: string } = {}) => {
		const start = before === undefined ? 0 : Number.parseInt(before.split('|')[1].slice(6), 10) + 1;
		return Array.from({ length: Math.max(0, Math.min(limit, total - start)) }, (_, index) =>
			traceSummary({ traceId: `trace-${start + index}`, lastActivityAt: '2026-09-25T18:04:22Z' })
		);
	};
}

describe('createTracesStore', () => {
	it('asks one row past the page and keeps the page, so the probe never reaches a table', async () => {
		const loadTraces = vi.fn(async () => page(['a', 'b', 'c'], '2026-09-25T18:04:22Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });

		expect(store.showSkeletons).toBe(true);
		await store.refresh();

		// Two rows held, three requested: the third is the answer to "is there more" and
		// is not part of the window, nor the cursor the next page is built from.
		expect(loadTraces).toHaveBeenLastCalledWith({ limit: 3 });
		expect(store.traces.map((trace) => trace.traceId)).toEqual(['a', 'b']);
		expect(store.showSkeletons).toBe(false);
		expect(store.hasMore).toBe(true);
	});

	it('appends the next page from the cursor of the last trail it holds', async () => {
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a', 'b', 'c'], '2026-09-25T18:04:22Z'))
			.mockResolvedValueOnce(page(['c', 'd'], '2026-09-24T10:00:00Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();

		await store.loadMore();

		// The boundary is 'b', the last trail held — not 'c', the probe row.
		expect(loadTraces).toHaveBeenLastCalledWith({ limit: 3, before: '2026-09-25T18:04:22Z|b' });
		expect(store.traces.map((trace) => trace.traceId)).toEqual(['a', 'b', 'c', 'd']);
		expect(store.hasMore).toBe(false);
	});

	it('ends the history on a page that came back whole, where a full page used to promise more', async () => {
		// Four trails, two per page: the last page is exactly full, and inferring "more"
		// from a full page would offer a third request that returns nothing.
		const loadTraces = vi.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>(
			pagedHistory(4)
		);
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();
		await store.loadMore();

		expect(store.traces).toHaveLength(4);
		expect(store.hasMore).toBe(false);
	});

	it('keeps paging past the point a bounded cursor scan used to stop at', async () => {
		const loadTraces = vi.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>(
			pagedHistory(20)
		);
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();

		for (let click = 0; click < 15; click++) {
			await store.loadMore();
		}

		expect(store.traces).toHaveLength(20);
		expect(store.hasMore).toBe(false);
	});

	it('pages from the same boundary after a poll brings new trails, because a cursor names a trail', async () => {
		const loadTraces = vi
			.fn<(page: { limit?: number; before?: string }) => Promise<TraceSummary[]>>()
			.mockResolvedValueOnce(page(['a', 'b', 'c'], '2026-09-25T18:04:22Z'))
			.mockResolvedValueOnce(page(['z', 'a', 'b'], '2026-09-26T09:00:00Z'))
			.mockResolvedValueOnce(page(['c', 'd'], '2026-09-24T10:00:00Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();
		await store.refresh();
		expect(store.traces.map((trace) => trace.traceId)).toEqual(['z', 'a', 'b']);

		await store.loadMore();

		// 'z' arrived and pushed 'a' and 'b' down the order, and the boundary is still
		// 'b'. An offset would have had to be corrected by hand to match, and would have
		// re-served a row the operator was already looking at.
		expect(loadTraces).toHaveBeenLastCalledWith({ limit: 3, before: '2026-09-25T18:04:22Z|b' });
		expect(store.traces.map((trace) => trace.traceId)).toEqual(['z', 'a', 'b', 'c', 'd']);
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
			.mockResolvedValueOnce(page(['a', 'b', 'c'], '2026-09-25T18:04:22Z'))
			.mockResolvedValueOnce(page(['c', 'd', 'e'], '2026-09-24T10:00:00Z'))
			.mockResolvedValueOnce(page(['z', 'a', 'b'], '2026-09-26T09:00:00Z'));
		const store = createTracesStore({ loadTraces, pageSize: 2, pollIntervalMs: 0 });
		await store.refresh();
		await store.loadMore();
		expect(store.traces).toHaveLength(4);

		await store.refresh();

		expect(store.traces.map((trace) => trace.traceId)).toEqual(['z', 'a', 'b', 'c', 'd']);
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
			.mockResolvedValueOnce(page(['a', 'b', 'c'], '2026-09-25T18:04:22Z'))
			.mockImplementationOnce(async () => {
				await gate;
				return page(['c', 'd'], '2026-09-24T10:00:00Z');
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
