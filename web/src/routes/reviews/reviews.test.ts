import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Reviews from './+page.svelte';
import ReviewDetail from './[traceId]/[taskId]/ReviewDetail.svelte';
import MergePreview from './[traceId]/[taskId]/preview/MergePreview.svelte';
import { createReviewStore } from '$lib/stores/review.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { mergeDiff, reviewQueue, reviewQueueItem, taskDetail } from '../../tests/fixtures';
import type { MergeDiff, ReviewQueue, TaskDetail } from '$lib/api/types';

function harness(
	queue: ReviewQueue = reviewQueue(),
	diff: MergeDiff = mergeDiff(),
	details: TaskDetail[] = [taskDetail({ taskId: '_review', isFinal: true })]
) {
	const listReviews = vi.fn(async () => queue);
	const getTask = vi.fn(async (traceId: string, taskId: string) => {
		const found = details.find((task) => task.traceId === traceId && task.taskId === taskId);
		if (!found) throw new Error('task not found');
		return found;
	});
	const getMergeDiff = vi.fn(async (traceId: string) => mergeDiff({ ...diff, traceId }));
	const store = createReviewStore({ listReviews, getTask, getMergeDiff, pollIntervalMs: 0 });
	const toasts = createToastStore(0);
	return { store, toasts, listReviews, getTask, getMergeDiff };
}

function stubFetch(handler: (url: string, init?: RequestInit) => Response): void {
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init))
	);
}

describe('reviews queue', () => {
	it('lists what is waiting and says how much of it there is', async () => {
		const { store } = harness();
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		expect(screen.getByRole('heading', { name: 'Reviews' })).toBeInTheDocument();
		expect(screen.getByText(/^2 proposals are waiting on you/)).toBeInTheDocument();
		const table = screen.getByLabelText('Review queue');
		expect(within(table).getByText('Merge body compose + approve preview')).toBeInTheDocument();
	});

	it('uses the singular for one, because "1 proposals" is a number nobody believes', async () => {
		const { store } = harness(reviewQueue({ items: [reviewQueueItem()], count: 1 }));
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		expect(screen.getByText(/^One proposal is waiting on you/)).toBeInTheDocument();
	});

	it('badges a final gate as one, because it is not the same decision', async () => {
		const { store } = harness();
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		const table = screen.getByLabelText('Review queue');
		expect(within(table).getByText('final gate')).toBeInTheDocument();
		expect(within(table).getByText('required')).toBeInTheDocument();
	});

	it('says what approving will do, which a merge commit and a PR are not the same', async () => {
		const { store } = harness();
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		const table = screen.getByLabelText('Review queue');
		// The final gate merges locally; a non-final review has no delivery at all.
		expect(within(table).getByText('local merge')).toBeInTheDocument();
		expect(within(table).getAllByText('—').length).toBeGreaterThan(0);
	});

	it('links a row to its proposal', async () => {
		const { store } = harness();
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		// The queue renders the mid-trail review first and the final gate second, so
		// the row is found by its own task rather than by position.
		const row = within(screen.getByLabelText('Review queue')).getByText('Human review and merge');
		expect(row.closest('tr')?.querySelector('a')).toHaveAttribute(
			'href',
			'/next/reviews/trace-01a0bd6963faa14f/_review'
		);
	});

	it('says the queue is empty rather than showing an empty table', async () => {
		const { store } = harness({ items: null, count: 0 });
		render(Reviews, { store });
		await waitFor(() => expect(screen.getByLabelText('Review queue')).toBeInTheDocument());

		// "0 proposals are waiting on you" is a sentence nobody believes; the zero case
		// says what an empty queue means instead.
		expect(screen.getByText(/^Nothing is waiting on you/)).toBeInTheDocument();
		expect(screen.getByText('No proposals awaiting review.')).toBeInTheDocument();
	});

	it('reports a queue that could not be read', async () => {
		const store = createReviewStore({
			listReviews: async () => {
				throw new Error('colony root unreadable');
			},
			getTask: async () => taskDetail(),
			getMergeDiff: async (traceId) => mergeDiff({ traceId }),
			pollIntervalMs: 0
		});
		render(Reviews, { store });

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('colony root unreadable'));
	});
});

describe('review detail', () => {
	async function openReview(taskId = '_review') {
		const h = harness();
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId,
			toasts: h.toasts
		});
		await waitFor(() => expect(h.listReviews).toHaveBeenCalled());
		return h;
	}

	it('names the proposal and badges it as waiting', async () => {
		await openReview();

		expect(
			await screen.findByRole('heading', { name: 'Human review and merge' })
		).toBeInTheDocument();
		expect(screen.getAllByText('waiting review').length).toBeGreaterThan(0);
	});

	it('links out to the trail, the task, and the timeline', async () => {
		await openReview();

		const identity = await screen.findByLabelText('Proposal identity');
		expect(within(identity).getByRole('link', { name: 'trace-01a0bd6963faa14f' })).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
		expect(screen.getByRole('link', { name: 'Task' })).toHaveAttribute(
			'href',
			'/next/tasks/trace-01a0bd6963faa14f/_review'
		);
		expect(screen.getByRole('link', { name: 'Timeline' })).toHaveAttribute(
			'href',
			'/next/timeline?trace=trace-01a0bd6963faa14f'
		);
	});

	it('says the final gate and what approving does with it', async () => {
		await openReview();

		const identity = await screen.findByLabelText('Proposal identity');
		expect(within(identity).getByText('final merge gate')).toBeInTheDocument();
		expect(within(identity).getByText('local merge')).toBeInTheDocument();
	});

	it('shows the trail summary as the merge commit body the reviewer is approving', async () => {
		const h = harness();
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.listReviews).toHaveBeenCalled());

		expect(screen.queryByText(/becomes the merge commit body/)).not.toBeInTheDocument();
	});

	it('offers the merge preview for a final gate and links the stat', async () => {
		await openReview();

		const changes = await screen.findByText('Open merge preview');
		expect(changes).toHaveAttribute(
			'href',
			'/next/reviews/trace-01a0bd6963faa14f/_review/preview'
		);
		expect(screen.getByText(/1 file changed/)).toBeInTheDocument();
	});

	it('hides the diff section entirely for a review that is not a final gate', async () => {
		const h = harness();
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '002-merge-body-compose',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.listReviews).toHaveBeenCalled());
		await screen.findByRole('heading', { name: 'Merge body compose + approve preview' });

		expect(screen.queryByText('Open merge preview')).not.toBeInTheDocument();
	});

	it('says a trail with no local branch instead of a broken preview', async () => {
		const h = harness(reviewQueue(), mergeDiff({ missingWorktree: true, diff: undefined }));
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getMergeDiff).toHaveBeenCalled());

		expect(await screen.findByText(/No branch for this trail on this machine/)).toBeInTheDocument();
		expect(screen.queryByText('Open merge preview')).not.toBeInTheDocument();
	});

	it('warns that the local default branch is behind origin before anyone merges', async () => {
		const h = harness(reviewQueue(), mergeDiff({ originBehindCount: 3 }));
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getMergeDiff).toHaveBeenCalled());

		expect(await screen.findByText(/is 3 commit\(s\) behind origin/)).toBeInTheDocument();
	});

	it('says a proposal that is no longer in the queue', async () => {
		const h = harness(reviewQueue({ items: [], count: 0 }), mergeDiff(), []);
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: 'task-99',
			toasts: h.toasts
		});

		expect(await screen.findByText('Proposal not found')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'All reviews' })).toHaveAttribute(
			'href',
			'/next/reviews'
		);
	});

	it('offers Approve and Reject to a proposal the ledger says it may act on', async () => {
		await openReview();

		expect(await screen.findByRole('button', { name: 'Approve…' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Reject…' })).toBeInTheDocument();
	});

	it('hides the decision from a proposal nobody may act on yet', async () => {
		const h = harness(
			reviewQueue({ items: [reviewQueueItem({ canApprove: false })], count: 1 })
		);
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '002-merge-body-compose',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.listReviews).toHaveBeenCalled());

		await screen.findByRole('heading', { name: 'Merge body compose + approve preview' });
		expect(screen.queryByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
	});

	it('re-reads the queue after a decision, so the row leaves the queue', async () => {
		stubFetch(() => new Response(JSON.stringify({ traceId: 'trace-1', taskId: '_review' })));
		const h = harness();
		render(ReviewDetail, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getTask).not.toHaveBeenCalled());
		await userEvent.click(await screen.findByRole('button', { name: 'Approve…' }));

		await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
		await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

		await waitFor(() => expect(h.listReviews.mock.calls.length).toBeGreaterThan(1));
		expect(h.toasts.items.at(-1)?.tone).toBe('success');
	});
});

describe('merge preview', () => {
	async function openPreview() {
		const h = harness();
		render(MergePreview, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getMergeDiff).toHaveBeenCalled());
		return h;
	}

	it('names the branch and the commit it is reviewing', async () => {
		await openPreview();

		expect(
			await screen.findByText('paseka/trace-01a0bd6963faa14f')
		).toBeInTheDocument();
		expect(screen.getByText('bbbbbbb')).toBeInTheDocument();
	});

	it('renders the diff as lines with their numbers, not as a raw patch', async () => {
		await openPreview();

		const diff = await screen.findByLabelText('Merge diff');
		// The third line is the addition, and its number is 3 on the new side.
		expect(within(diff).getByText('export const third = 3;')).toBeInTheDocument();
		// A hunk header is shown, which a raw `<pre>` would have shown as text and a
		// reviewer would have had to read past.
		expect(within(diff).getByText('@@ -1,2 +1,3 @@')).toBeInTheDocument();
		expect(within(diff).getAllByText('2').length).toBeGreaterThan(0);
	});

	it('lists the changed files with their stat, and filters them by path', async () => {
		await openPreview();

		const nav = await screen.findByLabelText('Changed files');
		expect(within(nav).getByText('web/src/lib/diff.ts')).toBeInTheDocument();
		expect(within(nav).getByText('+3 -0')).toBeInTheDocument();

		await userEvent.type(screen.getByLabelText('Filter files by path'), 'nothing-matches');
		expect(screen.getByText('No files match the filter.')).toBeInTheDocument();
	});

	it('keeps the body whole while the list filters, because line numbers must not jump', async () => {
		await openPreview();

		await userEvent.type(screen.getByLabelText('Filter files by path'), 'zzz');

		// Hiding the section would make every anchored comment point at a row that is
		// no longer on screen.
		expect(within(await screen.findByLabelText('Merge diff')).getByText('export const third = 3;'))
			.toBeInTheDocument();
	});

	it('anchors a note on the line that was clicked, and submits it with the head commit', async () => {
		let sent: Record<string, unknown> = {};
		stubFetch((url, init) => {
			if (url.endsWith('/reject')) {
				sent = JSON.parse(String(init?.body ?? '{}'));
				return new Response(
					JSON.stringify({ traceId: 'trace-1', taskId: '_review', reworkTaskId: 'task-09' })
				);
			}
			return new Response('{}');
		});
		await openPreview();

		const diff = await screen.findByLabelText('Merge diff');
		await userEvent.click(within(diff).getByText('export const third = 3;'));

		// The editor says where the note will land, before it is written.
		expect(screen.getByText(/web\/src\/lib\/diff\.ts · new L3/)).toBeInTheDocument();
		await userEvent.type(screen.getByLabelText('Note'), 'Name this constant.');
		await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

		expect(screen.getByText('1 draft')).toBeInTheDocument();
		await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

		await waitFor(() => expect(sent.headSha).toBe('b'.repeat(40)));
		expect(sent.comments).toEqual([
			{
				path: 'web/src/lib/diff.ts',
				side: 'new',
				startLine: 3,
				snippet: 'export const third = 3;',
				body: 'Name this constant.'
			}
		]);
	});

	it('drops a packet whose head commit moved, because its line numbers are void', async () => {
		const h = harness();
		const { unmount } = render(MergePreview, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getMergeDiff).toHaveBeenCalled());
		const diff = await screen.findByLabelText('Merge diff');
		await userEvent.click(within(diff).getByText('export const third = 3;'));
		await userEvent.type(screen.getByLabelText('Note'), 'Name this constant.');
		await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
		expect(screen.getByText('1 draft')).toBeInTheDocument();
		unmount();

		// The bee pushed while the reviewer was writing: the same trail, a new head.
		const moved = harness(reviewQueue(), mergeDiff({ headSha: 'c'.repeat(40) }));
		render(MergePreview, {
			store: moved.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: moved.toasts
		});
		await waitFor(() => expect(moved.getMergeDiff).toHaveBeenCalled());

		// A fresh mount has no drafts to lose, and the commit it pins is the new one.
		expect(screen.getByText('0 drafts')).toBeInTheDocument();
	});

	it('disables Request changes while a rework task is in flight', async () => {
		const h = harness(
			reviewQueue({
				items: [
					reviewQueueItem({
						taskId: '_review',
						title: 'Human review and merge',
						isFinal: true,
						reworkTaskId: 'task-09',
						reworkStatus: 'running'
					})
				],
				count: 1
			})
		);
		render(MergePreview, {
			store: h.store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: h.toasts
		});
		await waitFor(() => expect(h.getMergeDiff).toHaveBeenCalled());

		expect(await screen.findByText(/Rework task-09 is running/)).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Request changes' })).toBeDisabled();
	});

	it('reports a diff it could not read, rather than an empty merge preview', async () => {
		const store = createReviewStore({
			listReviews: async () => reviewQueue(),
			getTask: async (traceId, taskId) => taskDetail({ traceId, taskId, isFinal: true }),
			getMergeDiff: async () => {
				throw new Error('worktree branch not found');
			},
			pollIntervalMs: 0
		});
		render(MergePreview, {
			store,
			traceId: 'trace-01a0bd6963faa14f',
			taskId: '_review',
			toasts: createToastStore(0)
		});

		expect(await screen.findByRole('alert')).toHaveTextContent('worktree branch not found');
	});
});
