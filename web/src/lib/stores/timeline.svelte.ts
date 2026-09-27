import { listEvents as requestEvents } from '$lib/api/client';
import type { EventFeedItem, EventFilters } from '$lib/api/types';

/** Only the fields the server actually filters on; anything else is dropped. */
export type TimelineFilterPatch = EventFilters;

/**
 * The cadences the operator can pick, in seconds. `null` is Manual and is the
 * default: watching a trail is a choice an operator makes by arriving at a timer,
 * and every tick reads up to fifty trail directories, so a feed that moved on its
 * own would cost more than it is worth to somebody who only came to read it.
 */
export const timelinePollIntervals = [5, 10, 15, 60] as const;

export type TimelinePollSeconds = (typeof timelinePollIntervals)[number];

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
 * The event feed behind `/next/timeline`. It is the one route-scoped store whose
 * poll is the operator's to arm: the feed is cursor-paginated history, so
 * prepending new events to a list someone is scrolling or has filtered is worse
 * than a deliberate refresh. A timer is offered, but its tick is the same reset
 * read Refresh already performs — the list is replaced, never appended to — so
 * opting in costs no scroll position and no row identity.
 *
 * One read is in flight at a time. A second Apply while the first is still
 * resolving would append two pages against one cursor and could repeat or skip
 * the boundary event, so an overlapping apply is refused rather than queued. The
 * same guard drops a tick that lands inside an Apply, which is the overlap that
 * would matter.
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
	let pollSeconds = $state<TimelinePollSeconds | null>(null);
	/**
	 * Reactive because three buttons bind `disabled` to `busy`, and a plain variable
	 * behind a getter gives the template nothing to re-evaluate on: the binding would be
	 * computed once at mount and then only ever refresh by accident, whenever something
	 * else in the same subtree happened to change.
	 */
	let inFlight = $state(false);
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

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
		if (started) return;
		started = true;
		void read(true);
		armTimer();
	}

	function stop(): void {
		started = false;
		disarmTimer();
	}

	function disarmTimer(): void {
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
	}

	/**
	 * A tick is Refresh, so the arming of it is the same read and the same
	 * guarantees. Disarming first means switching cadences cannot leave the old
	 * interval running, and a `started` route keeps its choice across a pause.
	 */
	function armTimer(): void {
		disarmTimer();
		if (!started || pollSeconds === null) return;
		timer = setInterval(() => void read(true), pollSeconds * 1000);
	}

	/** `null` is Manual, which is also how the route's Refresh button says it. */
	function setPoll(seconds: TimelinePollSeconds | null): void {
		pollSeconds = seconds;
		armTimer();
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
		/** `null` while the feed is Manual, which is where it arrives. */
		get pollSeconds(): TimelinePollSeconds | null {
			return pollSeconds;
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
		setPoll,
		start,
		stop
	};
}

export type TimelineStore = ReturnType<typeof createTimelineStore>;
