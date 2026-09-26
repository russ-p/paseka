import { listEvents as requestEvents } from '$lib/api/client';
import type { EventFeedItem, EventFilters } from '$lib/api/types';

/** Only the fields the server actually filters on; anything else is dropped. */
export type TimelineFilterPatch = EventFilters;

interface TimelineStoreOptions {
	listEvents?: (filters: EventFilters, after?: string) => Promise<{
		items: EventFeedItem[];
		nextCursor?: string;
		hasMore: boolean;
	}>;
	/** The deep link from a trail's "Open timeline" scopes the feed before the first read. */
	initialFilters?: EventFilters;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function prune(filters: TimelineFilterPatch): EventFilters {
	const clean: EventFilters = {};
	for (const [key, value] of Object.entries(filters)) {
		const trimmed = typeof value === 'string' ? value.trim() : value;
		if (trimmed) clean[key as keyof EventFilters] = trimmed;
	}
	return clean;
}

/**
 * The event feed behind `/next/timeline`. It is the one route-scoped store that
 * does **not** poll: the feed is cursor-paginated history, and prepending new
 * events to a list an operator is scrolling or has filtered is worse than a
 * deliberate refresh. It reads on mount, on apply, on load-more, and on an
 * explicit refresh.
 *
 * One read is in flight at a time. A second Apply while the first is still
 * resolving would append two pages against one cursor and could repeat or skip
 * the boundary event, so an overlapping apply is refused rather than queued.
 */
export function createTimelineStore(options: TimelineStoreOptions = {}) {
	const readEvents =
		options.listEvents ?? ((filters, after) => requestEvents(filters, after));

	let filters = $state<EventFilters>(prune(options.initialFilters ?? {}));
	let items = $state<EventFeedItem[]>([]);
	let cursor = $state('');
	let hasMore = $state(false);
	let loading = $state(true);
	let loadingMore = $state(false);
	let error = $state('');
	let inFlight = false;

	async function read(reset: boolean): Promise<void> {
		if (inFlight) return;
		inFlight = true;
		if (reset) loading = true;
		else loadingMore = true;
		error = '';
		try {
			const page = await readEvents(filters, reset ? undefined : cursor);
			// A reset replaces the list; load-more appends, so the boundary event is
			// never shown twice.
			items = reset ? page.items : [...items, ...page.items];
			cursor = page.nextCursor ?? '';
			hasMore = page.hasMore;
		} catch (cause) {
			error = errorMessage(cause);
			// A failed load-more must not look like the end of the feed, so the
			// cursor and hasMore stay as they were and the button can be pressed again.
		} finally {
			loading = false;
			loadingMore = false;
			inFlight = false;
		}
	}

	/**
	 * Replace the filters and re-read from the newest page. Refused while a read
	 * is in flight, which is the overlap that would corrupt the cursor.
	 */
	async function apply(patch: TimelineFilterPatch): Promise<void> {
		if (inFlight) return;
		filters = prune(patch);
		items = [];
		cursor = '';
		hasMore = false;
		await read(true);
	}

	async function clear(): Promise<void> {
		await apply({});
	}

	async function loadMore(): Promise<void> {
		if (!hasMore || inFlight) return;
		await read(false);
	}

	function start(): void {
		void read(true);
	}

	function refresh(): Promise<void> {
		return read(true);
	}

	return {
		get filters(): EventFilters {
			return filters;
		},
		get items(): EventFeedItem[] {
			return items;
		},
		/** Skeletons only before the first payload; a later failure keeps the feed on screen. */
		get showSkeletons(): boolean {
			return loading && items.length === 0;
		},
		get loading(): boolean {
			return loading;
		},
		/** A second page is being appended, which is not the same as a reload. */
		get loadingMore(): boolean {
			return loadingMore;
		},
		get hasMore(): boolean {
			return hasMore;
		},
		/** True when any filter narrows the feed, so the filter panel says so. */
		get filtered(): boolean {
			return Object.keys(filters).length > 0;
		},
		get busy(): boolean {
			return inFlight;
		},
		get lastError(): string {
			return error;
		},
		apply,
		clear,
		loadMore,
		refresh,
		start
	};
}

export type TimelineStore = ReturnType<typeof createTimelineStore>;
