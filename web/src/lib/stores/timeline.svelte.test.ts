import { describe, expect, it, vi } from 'vitest';
import { createTimelineStore } from './timeline.svelte';
import { eventFeedItem, eventFeedPage } from '../../tests/fixtures';
import type { EventFeedItem, EventFilters } from '$lib/api/types';

type Page = { items: EventFeedItem[]; nextCursor?: string; hasMore: boolean };

/** A feed that answers with the pages it is given and records every call. */
function harness(
	pages: Page[] = [eventFeedPage()],
	initialFilters?: EventFilters
) {
	const listEvents = vi.fn(async () => pages.shift() ?? eventFeedPage([]));
	const store = createTimelineStore({ listEvents, initialFilters });
	return { store, listEvents };
}

describe('createTimelineStore', () => {
	it('reads once on mount and keeps the feed on screen through a later failure', async () => {
		const listEvents = vi
			.fn<() => Promise<Page>>()
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem()]))
			.mockRejectedValueOnce(new Error('nats: no responders'));
		const store = createTimelineStore({ listEvents });

		expect(store.showSkeletons).toBe(true);
		await store.start();
		expect(store.showSkeletons).toBe(false);
		expect(store.items).toHaveLength(1);

		await store.refresh();

		expect(store.lastError).toBe('nats: no responders');
		expect(store.items).toHaveLength(1);
	});

	it('scopes the first read with a deep link filter, so a trail link never loads the colony feed', async () => {
		const { store, listEvents } = harness([eventFeedPage()], { traceId: 'trace-01a0bd6963faa14f' });

		await store.start();

		expect(listEvents).toHaveBeenCalledWith({ traceId: 'trace-01a0bd6963faa14f' }, undefined);
		expect(store.filtered).toBe(true);
	});

	it('trims a filter and drops the empty ones, so a stray space is not a filter', async () => {
		const { store, listEvents } = harness();
		await store.start();

		await store.apply({ traceId: '  trace-1  ', bee: '   ', type: '' });

		expect(listEvents).toHaveBeenLastCalledWith({ traceId: 'trace-1' }, undefined);
		expect(store.filtered).toBe(true);
	});

	it('appends the next page against the cursor instead of replacing the feed', async () => {
		const listEvents = vi
			.fn<(_filters: EventFilters, after?: string) => Promise<Page>>()
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem()], { hasMore: true, nextCursor: 'cursor-1' }))
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem({ id: 'second' })]));
		const store = createTimelineStore({ listEvents });
		await store.start();

		await store.loadMore();

		expect(listEvents).toHaveBeenLastCalledWith({}, 'cursor-1');
		expect(store.items.map((item) => item.id)).toHaveLength(2);
		expect(store.hasMore).toBe(false);
	});

	it('keeps hasMore after a failed load-more, so the button is not a dead end', async () => {
		const listEvents = vi
			.fn<(_filters: EventFilters, after?: string) => Promise<Page>>()
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem()], { hasMore: true, nextCursor: 'cursor-1' }))
			.mockRejectedValueOnce(new Error('cursor expired'))
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem({ id: 'second' })]));
		const store = createTimelineStore({ listEvents });
		await store.start();

		await store.loadMore();
		expect(store.hasMore).toBe(true);
		expect(store.lastError).toBe('cursor expired');

		await store.loadMore();

		expect(store.items).toHaveLength(2);
		expect(store.lastError).toBe('');
	});

	it('refuses a second read while one is in flight, which would corrupt the cursor', async () => {
		const listEvents = vi.fn(async () => eventFeedPage());
		const store = createTimelineStore({ listEvents });

		await Promise.all([store.start(), store.loadMore(), store.refresh()]);

		expect(listEvents).toHaveBeenCalledTimes(1);
		expect(store.busy).toBe(false);
	});

	it('applies new filters from the newest page rather than appending to the old one', async () => {
		const listEvents = vi
			.fn<() => Promise<Page>>()
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem()], { hasMore: true, nextCursor: 'cursor-1' }))
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem({ id: 'filtered' })]));
		const store = createTimelineStore({ listEvents });
		await store.start();

		await store.apply({ type: 'VERIFICATION' });

		expect(store.items.map((item) => item.id)).toEqual(['filtered']);
		expect(store.hasMore).toBe(false);
	});

	it('clears back to the colony-wide feed', async () => {
		const { store, listEvents } = harness([eventFeedPage()], { traceId: 'trace-1', bee: 'scout' });
		await store.start();

		await store.clear();

		expect(listEvents).toHaveBeenLastCalledWith({}, undefined);
		expect(store.filtered).toBe(false);
	});

	it('separates a first load from appending a page, which are not the same wait', async () => {
		const listEvents = vi
			.fn<(_filters: EventFilters, after?: string) => Promise<Page>>()
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem()], { hasMore: true, nextCursor: 'cursor-1' }))
			.mockResolvedValueOnce(eventFeedPage([eventFeedItem({ id: 'second' })]));
		const store = createTimelineStore({ listEvents });

		const first = store.start();
		expect(store.loadingMore).toBe(false);
		await first;
		expect(store.loading).toBe(false);

		await store.loadMore();
		expect(store.loadingMore).toBe(false);
	});
});
