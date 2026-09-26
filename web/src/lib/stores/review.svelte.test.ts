import { describe, expect, it, vi } from 'vitest';
import { createReviewStore } from './review.svelte';
import { mergeDiff, reviewQueue, taskDetail } from '../../tests/fixtures';
import type { ReviewQueue, TaskDetail } from '$lib/api/types';

function harness(
	queue: ReviewQueue = reviewQueue(),
	details: TaskDetail[] = [taskDetail({ taskId: '_review', isFinal: true })]
) {
	const listReviews = vi.fn(async () => queue);
	const getTask = vi.fn(async (traceId: string, taskId: string) => {
		const found = details.find((task) => task.traceId === traceId && task.taskId === taskId);
		if (!found) throw new Error('task not found');
		return found;
	});
	const getMergeDiff = vi.fn(async (traceId: string) => mergeDiff({ traceId }));
	const store = createReviewStore({ listReviews, getTask, getMergeDiff, pollIntervalMs: 0 });
	return { store, listReviews, getTask, getMergeDiff };
}

describe('createReviewStore', () => {
	it('reads the queue once on mount and keeps the server count', async () => {
		const { store, listReviews } = harness();
		expect(store.showSkeletons).toBe(true);

		await store.start();
		await store.start();

		expect(listReviews).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
		// The count is the server's own figure, which can exceed the items it sent.
		expect(store.count).toBe(2);
	});

	it('answers a queue whose items came back null', async () => {
		// Go marshals a nil slice as null, and a colony with nothing to review sends
		// exactly that.
		const { store } = harness({ items: null, count: 0 });
		await store.start();

		expect(store.items).toEqual([]);
		expect(store.count).toBe(0);
	});

	it('selects from the queue without a second request', async () => {
		const { store, getTask } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', '002-merge-body-compose');

		// The queue item carries everything this page shows, including the delivery
		// and PR fields a plain task detail leaves out.
		expect(getTask).not.toHaveBeenCalled();
		expect(store.current?.title).toBe('Merge body compose + approve preview');
	});

	it('reads the diff for a final gate and skips it for a review that is not one', async () => {
		const { store, getMergeDiff } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', '_review');
		expect(getMergeDiff).toHaveBeenCalledWith('trace-01a0bd6963faa14f');
		expect(store.diff?.headSha).toBe('b'.repeat(40));

		getMergeDiff.mockClear();
		await store.select('trace-01a0bd6963faa14f', '002-merge-body-compose');
		// A mid-trail review has no worktree diff of its own to show.
		expect(getMergeDiff).not.toHaveBeenCalled();
	});

	it('reads one diff per trail, because the branch belongs to the trail', async () => {
		const { store, getMergeDiff } = harness();
		await store.start();

		await store.select('trace-01a0bd6963faa14f', '_review');
		await store.select('trace-01a0bd6963faa14f', '003-merge-body-preview');

		expect(getMergeDiff).toHaveBeenCalledTimes(1);
	});

	it('drops a slow diff that lands after the selection moved on', async () => {
		// Two clicks faster than the first read returns would otherwise paint one
		// trail's diff under another's heading.
		let releaseFirst = (): void => {};
		const first = new Promise<void>((resolve) => {
			releaseFirst = resolve;
		});
		const store = createReviewStore({
			listReviews: async () => reviewQueue(),
			getTask: async (traceId, taskId) => taskDetail({ traceId, taskId, isFinal: true }),
			getMergeDiff: async (traceId) => {
				if (traceId === 'trace-01a0bd6963faa14f') await first;
				return mergeDiff({ traceId, branch: `branch/${traceId}` });
			},
			pollIntervalMs: 0
		});
		await store.start();

		const slow = store.select('trace-01a0bd6963faa14f', '_review');
		await store.select('trace-019f8d632b01bb1f', '_review');
		expect(store.diff?.branch).toBe('branch/trace-019f8d632b01bb1f');

		releaseFirst();
		await slow;
		expect(store.diff?.branch).toBe('branch/trace-019f8d632b01bb1f');
	});

	it('re-reads the diff on demand, which is how a review notices a new commit', async () => {
		const { store, getMergeDiff } = harness();
		await store.start();
		await store.select('trace-01a0bd6963faa14f', '_review');

		await store.reloadDiff();

		expect(getMergeDiff).toHaveBeenCalledTimes(2);
	});

	it('keeps a selected item readable across a poll that no longer lists it', async () => {
		// The operator is still reading it; emptying the page under them because a
		// queue moved would be worse than showing a proposal that has left the queue.
		const listReviews = vi
			.fn<() => Promise<ReviewQueue>>()
			.mockResolvedValueOnce(reviewQueue())
			.mockResolvedValueOnce({ items: [], count: 0 });
		const store = createReviewStore({
			listReviews,
			getTask: async () => taskDetail(),
			getMergeDiff: async (traceId) => mergeDiff({ traceId }),
			pollIntervalMs: 0
		});
		await store.start();
		await store.select('trace-01a0bd6963faa14f', '_review');

		await store.refresh();

		expect(store.current?.taskId).toBe('_review');
	});

	it('keeps the queue on screen through a failed read', async () => {
		const listReviews = vi
			.fn<() => Promise<ReviewQueue>>()
			.mockResolvedValueOnce(reviewQueue())
			.mockRejectedValueOnce(new Error('colony root unreadable'));
		const store = createReviewStore({
			listReviews,
			getTask: async () => taskDetail(),
			getMergeDiff: async (traceId) => mergeDiff({ traceId }),
			pollIntervalMs: 0
		});
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('colony root unreadable');
		expect(store.items).toHaveLength(2);
	});

	it('reports a diff it could not read, apart from the queue', async () => {
		const store = createReviewStore({
			listReviews: async () => reviewQueue(),
			getTask: async (traceId, taskId) => taskDetail({ traceId, taskId, isFinal: true }),
			getMergeDiff: async () => {
				throw new Error('worktree branch not found');
			},
			pollIntervalMs: 0
		});
		await store.start();

		await store.select('trace-01a0bd6963faa14f', '_review');

		expect(store.diffError).toBe('worktree branch not found');
		expect(store.diff).toBeNull();
		// The proposal itself is still readable, which is the point of reporting the
		// two failures apart.
		expect(store.current).not.toBeNull();
	});

	it('fetches the task itself when a deep link names a proposal the queue dropped', async () => {
		const { store, getTask } = harness(reviewQueue({ items: [], count: 0 }));
		await store.start();

		await store.select('trace-01a0bd6963faa14f', '_review');

		expect(getTask).toHaveBeenCalledWith('trace-01a0bd6963faa14f', '_review');
	});

	it('drops the previous trail\u2019s diff when the selection is not a final gate', async () => {
		// A mid-trail review has no diff of its own, so leaving the last final gate's
		// patch on screen would put one proposal's code under another proposal's
		// heading — the kind of mistake a reviewer approves straight through.
		const h = harness(
			reviewQueue(),
			[taskDetail({ taskId: '_review', isFinal: true }), taskDetail({ taskId: '_step' })]
		);
		await h.store.start();
		await h.store.select('trace-01a0bd6963faa14f', '_review');
		expect(h.store.diff).not.toBeNull();
		const reads = h.getMergeDiff.mock.calls.length;

		await h.store.select('trace-01a0bd6963faa14f', '_step');

		expect(h.store.diff).toBeNull();
		expect(h.store.diffLoading).toBe(false);
		// And it did not go looking for a diff the gate cannot have.
		expect(h.getMergeDiff).toHaveBeenCalledTimes(reads);
	});

	it('keeps the diff on screen through a re-read of the same trail', async () => {
		// The comments panel is mounted beside the diff, so blanking the diff on a
		// refresh would unmount it and take every draft with it. A reviewer must not
		// lose their notes because the agent happened to push.
		let head = 'b'.repeat(40);
		const store = createReviewStore({
			listReviews: vi.fn(async () => reviewQueue()),
			getTask: vi.fn(async () => taskDetail({ taskId: '_review', isFinal: true })),
			getMergeDiff: vi.fn(async (traceId: string) => mergeDiff({ traceId, headSha: head })),
			pollIntervalMs: 0
		});
		await store.start();
		await store.select('trace-01a0bd6963faa14f', '_review');
		const first = store.diff;
		expect(first?.headSha).toBe('b'.repeat(40));

		head = 'c'.repeat(40);
		const pending = store.reloadDiff();

		// Mid-flight: still the old diff, and the loading flag the page shows its
		// "re-reading" hint from.
		expect(store.diff).toBe(first);
		expect(store.diffLoading).toBe(true);
		await pending;

		expect(store.diff?.headSha).toBe('c'.repeat(40));
		expect(store.diffLoading).toBe(false);
	});

	it('drops a diff that arrives after the selection moved on', async () => {
		let release: (() => void) | undefined;
		const store = createReviewStore({
			listReviews: vi.fn(async () => reviewQueue()),
			getTask: vi.fn(async () => taskDetail({ taskId: '_review', isFinal: true })),
			getMergeDiff: vi.fn(
				(traceId: string) =>
					new Promise<ReturnType<typeof mergeDiff>>((resolve) => {
						release = () => resolve(mergeDiff({ traceId }));
					})
			),
			pollIntervalMs: 0
		});
		await store.start();
		const pending = store.select('trace-01a0bd6963faa14f', '_review');
		await Promise.resolve();

		// The reviewer gives up on this gate and opens another before the read lands.
		await store.clearDiff();
		release?.();
		await pending;

		// A late response must not paint under a page that has moved on.
		expect(store.diff).toBeNull();
	});

	it('ignores a select with no ids', async () => {
		const { store, getTask } = harness();
		await store.start();

		await store.select('', '');

		expect(getTask).not.toHaveBeenCalled();
		expect(store.current).toBeNull();
	});

	it('polls, because a review queue is what an operator waits on', async () => {
		vi.useFakeTimers();
		try {
			const listReviews = vi.fn(async () => reviewQueue());
			const store = createReviewStore({
				listReviews,
				getTask: async () => taskDetail(),
				getMergeDiff: async (traceId) => mergeDiff({ traceId }),
				pollIntervalMs: 5000
			});

			store.start();
			await vi.advanceTimersByTimeAsync(10_000);
			expect(listReviews).toHaveBeenCalledTimes(3);

			store.stop();
			await vi.advanceTimersByTimeAsync(10_000);
			expect(listReviews).toHaveBeenCalledTimes(3);
		} finally {
			vi.useRealTimers();
		}
	});
});
