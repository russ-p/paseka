import { getMergeDiff as requestMergeDiff, getTask, listReviews } from '$lib/api/client';
import type { MergeDiff, ReviewQueue, ReviewQueueItem, TaskDetail } from '$lib/api/types';

interface ReviewStoreOptions {
	listReviews?: () => Promise<ReviewQueue>;
	getTask?: (traceId: string, taskId: string) => Promise<TaskDetail>;
	getMergeDiff?: (traceId: string) => Promise<MergeDiff>;
	/** A review queue is what an operator is waiting on, so this page does poll. */
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

export function createReviewStore(options: ReviewStoreOptions = {}) {
	const readQueue = options.listReviews ?? listReviews;
	const readTask = options.getTask ?? getTask;
	const readDiff = options.getMergeDiff ?? requestMergeDiff;
	const pollIntervalMs = options.pollIntervalMs ?? 5000;

	let queue = $state<ReviewQueue>({ items: [], count: 0 });
	let selectedTraceId = $state('');
	let selectedTaskId = $state('');
	/** The queue item, or the task fetched when the queue no longer lists it. */
	let current = $state<ReviewQueueItem | TaskDetail | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');

	let diff = $state<MergeDiff | null>(null);
	let diffTraceId = $state('');
	let diffLoading = $state(false);
	let diffError = $state('');
	/**
	 * Guards a diff read against the selection moving on: two clicks in quick
	 * succession would otherwise let the slower response paint under the wrong task.
	 */
	let diffToken = 0;

	let loading = $state(true);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;
	/**
	 * The first queue read, so a select that races it waits rather than concluding
	 * the proposal is not in the queue and fetching the task as well. A deep link
	 * would otherwise cost two requests on every load, and a proposal that *was* in
	 * the queue would briefly render as a task-only view.
	 */
	let ready: Promise<void> | undefined;

	async function refresh(): Promise<void> {
		try {
			queue = await readQueue();
			error = '';
			// A selected item keeps its identity across a poll even once it has left
			// the queue, because the operator is still reading it. A 404 turns into a
			// readable message rather than a page that empties under them.
			if (selectedTraceId !== '' && selectedTaskId !== '') {
				const found = items().find(
					(item) => item.traceId === selectedTraceId && item.taskId === selectedTaskId
				);
				if (found) current = found;
			}
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/** Plain functions rather than `$derived`, like every other store here. */
	function items(): ReviewQueueItem[] {
		return queue.items ?? [];
	}

	function count(): number {
		return queue.count ?? items().length;
	}

	function find(traceId: string, taskId: string): ReviewQueueItem | null {
		return (
			items().find((item) => item.traceId === traceId && item.taskId === taskId) ?? null
		);
	}

	/**
	 * The diff is only a thing for a final gate, and only for its own trail.
	 *
	 * A re-read of the *same* trail deliberately leaves the old diff in place while it
	 * is in flight. Clearing it would blank the patch a reviewer is reading, and — since
	 * the comments panel is mounted beside the diff — would unmount that panel too and
	 * take every draft with it, which is the one thing a review must never do because
	 * the agent happened to push. A different trail does get a blank, because its diff
	 * is a different patch.
	 */
	async function loadDiff(traceId: string, force = false): Promise<void> {
		if (traceId === '' || (diffTraceId === traceId && !force)) return;
		const token = ++diffToken;
		const sameTrail = diffTraceId === traceId;
		diffTraceId = traceId;
		if (!sameTrail) diff = null;
		diffError = '';
		diffLoading = true;
		try {
			const loaded = await readDiff(traceId);
			if (token !== diffToken) return;
			diff = loaded;
		} catch (cause) {
			if (token !== diffToken) return;
			diffError = errorMessage(cause);
		} finally {
			if (token === diffToken) diffLoading = false;
		}
	}

	async function select(traceId: string, taskId: string): Promise<void> {
		if (!traceId || !taskId) return;
		if (ready) await ready;
		selectedTraceId = traceId;
		selectedTaskId = taskId;
		detailError = '';
		const queued = find(traceId, taskId);
		if (queued) {
			// From the queue: no second request. It carries everything the review page
			// shows, including the delivery and PR fields a plain task detail leaves out.
			current = queued;
			detailLoading = false;
		} else {
			// A deep link, or an item approved from another tab: read the task itself.
			detailLoading = true;
			try {
				current = await readTask(traceId, taskId);
			} catch (cause) {
				current = null;
				detailError = errorMessage(cause);
			} finally {
				detailLoading = false;
			}
		}
		if (current?.isFinal) await loadDiff(traceId);
		else clearDiff();
	}

	/**
	 * Re-read the diff, which is how a review notices the bee pushed new commits. It
	 * does not clear what is on screen, so the panel a reviewer is annotating survives
	 * and can warn them instead of quietly losing their notes.
	 */
	async function reloadDiff(): Promise<void> {
		if (selectedTraceId === '') return;
		await loadDiff(selectedTraceId, true);
	}

	function clearDiff(): void {
		// Also invalidates an in-flight read, so a diff that was already on the wire for
		// the previous trail cannot land under a task that has no diff of its own.
		diffToken += 1;
		diffTraceId = '';
		diff = null;
		diffError = '';
		diffLoading = false;
	}

	function start(): void {
		if (started) return;
		started = true;
		ready = refresh();
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
		get items(): ReviewQueueItem[] {
			return items();
		},
		get count(): number {
			return count();
		},
		get current(): ReviewQueueItem | TaskDetail | null {
			return current;
		},
		get showSkeletons(): boolean {
			// Only the first read, on either outcome: a later failure keeps the last
			// queue on screen rather than replacing it with a skeleton.
			return loading;
		},
		get lastError(): string {
			return error;
		},
		get detailLoading(): boolean {
			return detailLoading;
		},
		get detailError(): string {
			return detailError;
		},
		get diff(): MergeDiff | null {
			return diff;
		},
		get diffLoading(): boolean {
			return diffLoading;
		},
		get diffError(): string {
			return diffError;
		},
		find,
		refresh,
		select,
		reloadDiff,
		clearDiff,
		start,
		stop
	};
}

export type ReviewStore = ReturnType<typeof createReviewStore>;
