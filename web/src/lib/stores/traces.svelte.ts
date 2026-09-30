import { listTraces as requestTraces, traceCursor } from '$lib/api/client';
import type { TraceSummary } from '$lib/api/types';

interface TracesStoreOptions {
	loadTraces?: (page: { limit?: number; before?: string }) => Promise<TraceSummary[]>;
	pageSize?: number;
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

function merge(fresh: TraceSummary[], known: TraceSummary[]): TraceSummary[] {
	/** A poll must not drop the older pages the operator pulled in. */
	const seen = new Set(fresh.map((trace) => trace.traceId));
	return [...fresh, ...known.filter((trace) => !seen.has(trace.traceId))];
}

/**
 * The trail history behind the Traces route. It is a route-scoped poller, not a
 * second dashboard poller: `consoleStatusStore` owns `/api/dashboard` and this
 * owns `/api/traces`, so the two never race over the same endpoint. The route
 * starts it on mount and stops it on unmount, so an idle console does not keep
 * polling the trail history.
 *
 * Paging is server-side by cursor, and this store owns all of it: the table it
 * feeds is unpaged, so the loaded window *is* the list. Filtering is the table's
 * job, over the window this store holds.
 */
export function createTracesStore(options: TracesStoreOptions = {}) {
	const readTraces = options.loadTraces ?? requestTraces;
	const pageSize = options.pageSize ?? 50;
	const pollIntervalMs = options.pollIntervalMs ?? 15000;

	let traces = $state<TraceSummary[]>([]);
	let loading = $state(true);
	let loadingMore = $state(false);
	let hasMore = $state(false);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	/**
	 * One row past the page, so "is there more" is answered by the response instead of
	 * guessed from a full page. A full page used to mean *probably* more, which was
	 * wrong exactly once per history — a total that was a whole multiple of the page
	 * size offered a button that returned nothing. The probe row is dropped by the
	 * caller and never reaches a table or a cursor.
	 */
	function nextPage(before?: string): Promise<TraceSummary[]> {
		return readTraces(before === undefined ? { limit: pageSize + 1 } : { limit: pageSize + 1, before });
	}

	async function refresh(): Promise<void> {
		try {
			const response = await nextPage();
			traces = merge(response.slice(0, pageSize), traces);
			hasMore = response.length > pageSize;
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function loadMore(): Promise<void> {
		if (loadingMore || !hasMore || traces.length === 0) return;
		loadingMore = true;
		try {
			// The cursor comes off the last trail the store holds, not off a row count.
			// A poll that prepends new trails does not move that trail, so the boundary
			// this page is built against is still the right one; an offset would have had
			// to be corrected by hand every time the colony produced something.
			const response = await nextPage(traceCursor(traces[traces.length - 1]));
			const older = response.slice(0, pageSize);
			hasMore = response.length > pageSize;
			traces = merge(traces, older);
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loadingMore = false;
		}
	}

	function start(): void {
		if (started) return;
		started = true;
		void refresh();
		if (pollIntervalMs > 0) {
			timer = setInterval(() => void refresh(), pollIntervalMs);
		}
	}

	function stop(): void {
		started = false;
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
	}

	return {
		get traces(): TraceSummary[] {
			return traces;
		},
		get loading(): boolean {
			return loading;
		},
		/** Skeletons only before the first payload; a later failure keeps the rows and shows the alert. */
		get showSkeletons(): boolean {
			return loading && traces.length === 0;
		},
		get loadingMore(): boolean {
			return loadingMore;
		},
		get hasMore(): boolean {
			return hasMore;
		},
		get lastError(): string {
			return error;
		},
		refresh,
		loadMore,
		start,
		stop
	};
}

export type TracesStore = ReturnType<typeof createTracesStore>;
