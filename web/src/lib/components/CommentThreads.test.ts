import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CommentThreads from './CommentThreads.svelte';
import { reviewQueueItem, taskDetail } from '../../tests/fixtures';
import type { DiffAnchor } from '$lib/diff';
import type { ReviewQueueItem, TaskDetail } from '$lib/api/types';

const HEAD = 'b'.repeat(40);

function anchor(overrides: Partial<DiffAnchor> = {}): DiffAnchor {
	return {
		path: 'web/src/lib/diff.ts',
		side: 'new',
		line: 3,
		snippet: 'export const third = 3;',
		...overrides
	};
}

function stubReject(result: () => Response): { sent: Record<string, unknown>[] } {
	const sent: Record<string, unknown>[] = [];
	vi.stubGlobal(
		'fetch',
		vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
			sent.push(JSON.parse(String(init?.body ?? '{}')));
			return result();
		})
	);
	return { sent };
}

function json(body: unknown): Response {
	return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } });
}

afterEach(() => {
	vi.unstubAllGlobals();
});

/**
 * `anchorOn` is the whole wiring contract with `DiffViewer`: the body owns the diff
 * and the panel owns the notes, so the page passes one in and neither component
 * reads the other's state. Driving it through the instance is what a page does.
 */
function mount(
	task: TaskDetail | ReviewQueueItem = reviewQueueItem({ taskId: '_review', isFinal: true }),
	headSha: string = HEAD
) {
	const onsettled = vi.fn();
	const view = render(CommentThreads, { task, headSha, onsettled });
	return { ...view, onsettled, instance: view.component };
}

describe('CommentThreads', () => {
	describe('anchoring a note', () => {
		it('opens an editor on the clicked line and says where the note will land', async () => {
			const { instance } = mount();

			expect(screen.queryByLabelText('Note')).not.toBeInTheDocument();
			instance.anchorOn(anchor());

			// The anchor is stated before the note is written, because a note written
			// against the wrong line is worth nothing.
			expect(await screen.findByText('web/src/lib/diff.ts · new L3')).toBeInTheDocument();
			expect(screen.getByLabelText('Note')).toBeInTheDocument();
		});

		it('widens to a range on a second click in the same file and side', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor({ line: 3, snippet: 'export const third = 3;' }));
			await screen.findByText('web/src/lib/diff.ts · new L3');
			instance.anchorOn(anchor({ line: 5, snippet: 'export const fifth = 5;' }));

			// Both ends are kept: a note about a block is written without dragging.
			expect(await screen.findByText('web/src/lib/diff.ts · new L3–5')).toBeInTheDocument();
		});

		it('takes the range whichever end the second click names', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor({ line: 9, snippet: 'const ninth = 9;' }));
			await screen.findByText('web/src/lib/diff.ts · new L9');
			instance.anchorOn(anchor({ line: 4, snippet: 'const fourth = 4;' }));

			expect(await screen.findByText('web/src/lib/diff.ts · new L4–9')).toBeInTheDocument();
		});

		it('starts a new note rather than a range when the side changes', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor({ side: 'new', line: 3 }));
			await screen.findByText('web/src/lib/diff.ts · new L3');
			instance.anchorOn(anchor({ side: 'old', line: 3 }));

			// Old and new line 3 are two different lines of two different versions, so
			// a range across them would name a span that does not exist in either.
			expect(await screen.findByText('web/src/lib/diff.ts · old L3')).toBeInTheDocument();
		});

		it('starts a new note rather than a range when the file changes', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor({ path: 'a.go', line: 3 }));
			await screen.findByText('a.go · new L3');
			instance.anchorOn(anchor({ path: 'b.go', line: 3 }));

			expect(await screen.findByText('b.go · new L3')).toBeInTheDocument();
		});

		it('clears the body of an abandoned note, so a half-written thought is not submitted', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			// Another file, because a second click in the same one widens the range
			// rather than starting over — see the two range cases above.
			instance.anchorOn(anchor({ path: 'web/src/lib/api.ts', snippet: 'export const Api = 1;' }));

			expect(await screen.findByText('web/src/lib/api.ts · new L3')).toBeInTheDocument();
			expect(screen.getByLabelText('Note')).toHaveValue('');
		});

		it('discards the editor on Cancel, keeping the drafts already saved', async () => {
			const { instance } = mount();

			instance.anchorOn(anchor({ line: 3 }));
			await userEvent.type(await screen.findByLabelText('Note'), 'First note.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			instance.anchorOn(anchor({ line: 4 }));
			await userEvent.type(await screen.findByLabelText('Note'), 'Second note.');

			await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

			expect(screen.queryByLabelText('Note')).not.toBeInTheDocument();
			expect(screen.getByText('1 draft')).toBeInTheDocument();
			expect(screen.getByText('First note.')).toBeInTheDocument();
		});
	});

	describe('drafts', () => {
		it('needs a body before it will save, because an empty note carries no instruction', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await screen.findByLabelText('Note');

			expect(screen.getByRole('button', { name: 'Add draft' })).toBeDisabled();
			await userEvent.type(screen.getByLabelText('Note'), '   ');
			expect(screen.getByRole('button', { name: 'Add draft' })).toBeDisabled();
		});

		it('counts drafts in the singular for one', async () => {
			const { instance } = mount();
			expect(screen.getByText('0 drafts')).toBeInTheDocument();

			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			expect(screen.getByText('1 draft')).toBeInTheDocument();
		});

		it('replaces a note on a range it already holds rather than listing it twice', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			// Clicking the same line again and saving over it is how a reviewer changes
			// their mind about one note, not how they write two notes about one line.
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name it better.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			expect(screen.getByText('1 draft')).toBeInTheDocument();
			expect(screen.getByText('Name it better.')).toBeInTheDocument();
			expect(screen.queryByText('Name this constant.')).not.toBeInTheDocument();
		});

		it('moves a draft that was edited onto the line it was re-aimed at', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			await userEvent.click(screen.getByRole('button', { name: 'Edit' }));
			await screen.findByRole('button', { name: 'Update draft' });

			// Clicking another line abandons the edit: the editor goes back to adding,
			// because a re-aimed note and a second note about another line are not the
			// same thing to say and the reviewer has not said which they meant.
			instance.anchorOn(anchor({ line: 7, snippet: 'const seventh = 7;' }));
			// The click widened the pending note rather than replacing it — one file, one
			// side — so the editor says the range and forgets it was updating a draft.
			expect(await screen.findByText('web/src/lib/diff.ts · new L3–7')).toBeInTheDocument();
			expect(screen.getByRole('button', { name: 'Add draft' })).toBeInTheDocument();
			expect(screen.getByLabelText('Note')).toHaveValue('');

			await userEvent.type(screen.getByLabelText('Note'), 'And name this one.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			// So both are kept: the edited note on line 3 and the new one over 3–7.
			expect(screen.getByText('2 drafts')).toBeInTheDocument();
			expect(screen.getByText('Name this constant.')).toBeInTheDocument();
			expect(screen.getByText('And name this one.')).toBeInTheDocument();
		});

		it('edits a draft in place and says it is updating rather than adding', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(screen.getByRole('button', { name: 'Edit' }));
			expect(await screen.findByRole('button', { name: 'Update draft' })).toBeInTheDocument();
			expect(screen.getByLabelText('Note')).toHaveValue('Name this constant.');

			await userEvent.clear(screen.getByLabelText('Note'));
			await userEvent.type(screen.getByLabelText('Note'), 'Third time.');
			await userEvent.click(screen.getByRole('button', { name: 'Update draft' }));

			expect(screen.getByText('Third time.')).toBeInTheDocument();
			expect(screen.queryByText('Name this constant.')).not.toBeInTheDocument();
		});

		it('deletes a draft by the line it is on, and the count follows', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(
				screen.getByRole('button', { name: 'Delete comment on web/src/lib/diff.ts line 3' })
			);

			expect(screen.queryByText('Name this constant.')).not.toBeInTheDocument();
			expect(screen.getByText('0 drafts')).toBeInTheDocument();
		});

		it('closes the editor when the draft being edited is deleted', async () => {
			const { instance } = mount();
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			await userEvent.click(screen.getByRole('button', { name: 'Edit' }));
			await screen.findByLabelText('Note');

			await userEvent.click(
				screen.getByRole('button', { name: 'Delete comment on web/src/lib/diff.ts line 3' })
			);

			// An editor with no note behind it would save a draft nobody can see.
			expect(screen.queryByLabelText('Note')).not.toBeInTheDocument();
		});
	});

	describe('sending the packet', () => {
		async function withDraft(body = 'Name this constant.'): Promise<ReturnType<typeof mount>> {
			const view = mount();
			view.instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), body);
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			return view;
		}

		it('sends a single-line note without a zero-width range', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review', reworkTaskId: 'task-09' }));
			await withDraft();

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			// `endLine` equal to `startLine` is omitted, because the rework prompt quotes
			// the range and "lines 3 to 3" reads as a mistake.
			expect(sent[0].comments).toEqual([
				{
					path: 'web/src/lib/diff.ts',
					side: 'new',
					startLine: 3,
					snippet: 'export const third = 3;',
					body: 'Name this constant.'
				}
			]);
		});

		it('sends a range with both of its ends', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			const { instance } = mount();
			instance.anchorOn(anchor({ line: 3, snippet: 'const third = 3;' }));
			await screen.findByLabelText('Note');
			instance.anchorOn(anchor({ line: 5, snippet: 'const fifth = 5;' }));
			await userEvent.type(await screen.findByLabelText('Note'), 'Rewrite this block.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0].comments).toEqual([
				{
					path: 'web/src/lib/diff.ts',
					side: 'new',
					startLine: 3,
					endLine: 5,
					// The far end's text, because that is the line the second click named.
					snippet: 'const fifth = 5;',
					body: 'Rewrite this block.'
				}
			]);
		});

		it('omits an empty snippet rather than sending a blank quote', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			const { instance } = mount();
			instance.anchorOn(anchor({ snippet: '' }));
			await userEvent.type(await screen.findByLabelText('Note'), 'Delete this line.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect((sent[0].comments as Record<string, unknown>[])[0]).not.toHaveProperty('snippet');
		});

		it('pins the packet to the head the notes were read against', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			await withDraft();

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			// Without the commit, a note's line numbers could not be checked against
			// anything and a rework would land on the wrong lines.
			expect(sent[0].headSha).toBe(HEAD);
		});

		it('sends no commit at all when the server never reported one', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			const { instance } = mount(reviewQueueItem({ taskId: '_review' }), '');
			instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0]).not.toHaveProperty('headSha');
		});

		it('sends a general note with no drafts, because a theme is a real answer', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			mount();

			expect(screen.getByRole('button', { name: 'Request changes' })).toBeDisabled();
			await userEvent.type(screen.getByLabelText('Overall'), 'Approach is wrong; use the existing seam.');

			expect(screen.getByRole('button', { name: 'Request changes' })).toBeEnabled();
			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0]).toEqual({
				feedback: 'Approach is wrong; use the existing seam.',
				headSha: HEAD,
				comments: []
			});
		});

		it('reports the outcome the server gave, and empties the panel', async () => {
			stubReject(() => json({ traceId: 't', taskId: '_review', message: 'Rework task-09 created.' }));
			const { onsettled } = await withDraft();

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			expect(onsettled).toHaveBeenCalledWith({ tone: 'warning', message: 'Rework task-09 created.' });
			// The packet is on its way, so holding it would offer to send it twice.
			expect(screen.getByText('0 drafts')).toBeInTheDocument();
			expect(screen.getByLabelText('Overall')).toHaveValue('');
		});

		it('keeps the drafts and says the reason when the send fails', async () => {
			stubReject(() => new Response('worktree is dirty', { status: 409 }));
			await withDraft();

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			// A reviewer's notes are their work, and a failed send is the one moment
			// losing them would be least forgivable.
			expect(await screen.findByRole('alert')).toHaveTextContent('worktree is dirty');
			expect(screen.getByText('1 draft')).toBeInTheDocument();
			expect(screen.getByText('Name this constant.')).toBeInTheDocument();
		});

		it('holds the send while a rework task from an earlier rejection is running', async () => {
			mount(reviewQueueItem({ taskId: '_review', reworkTaskId: 'task-09', reworkStatus: 'running' }));
			await userEvent.type(screen.getByLabelText('Overall'), 'Try again.');

			expect(screen.getByText(/Rework task-09 is running/)).toBeInTheDocument();
			// A second rework would collide with the first on the same worktree.
			expect(screen.getByRole('button', { name: 'Request changes' })).toBeDisabled();
		});

		it('says why a proposal bee that is not isolated cannot take a packet', async () => {
			mount(reviewQueueItem({ taskId: '_review', canRequestChanges: false }));

			expect(screen.getByText(/the proposal bee is not isolated/)).toBeInTheDocument();
			expect(screen.getByRole('button', { name: 'Request changes' })).toBeDisabled();
		});
	});

	describe('a head that moves under an open review', () => {
		it('keeps the notes, names both commits, and holds the send until they are re-read', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: '_review' }));
			const view = mount();
			view.instance.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			expect(screen.getByText('1 draft')).toBeInTheDocument();

			// The same mounted page sees the bee push, which is the only case that has
			// drafts to lose. A remount would pass whatever the code did.
			await view.rerender({ task: reviewQueueItem({ taskId: '_review' }), headSha: 'c'.repeat(40) });

			const warning = await screen.findByRole('alert');
			expect(warning).toHaveTextContent('The agent pushed while you were writing');
			expect(warning).toHaveTextContent('bbbbbbb');
			expect(warning).toHaveTextContent('ccccccc');
			expect(warning).toHaveTextContent('Your note is');
			expect(screen.getByText('1 draft')).toBeInTheDocument();
			expect(screen.getByText('Name this constant.')).toBeInTheDocument();
			expect(screen.getByRole('button', { name: 'Request changes' })).toBeDisabled();

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));
			expect(sent).toHaveLength(0);

			await userEvent.click(screen.getByRole('button', { name: "I've re-read the diff" }));

			expect(screen.queryByText(/The agent pushed while you were writing/)).not.toBeInTheDocument();
			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			// The commit that goes out is the one the reviewer acknowledged, not the
			// one the line numbers were read against.
			expect(sent[0].headSha).toBe('c'.repeat(40));
		});

		it('counts the notes it is holding in the warning', async () => {
			const view = mount();
			view.instance.anchorOn(anchor({ line: 3 }));
			await userEvent.type(await screen.findByLabelText('Note'), 'First.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));
			view.instance.anchorOn(anchor({ line: 4 }));
			await userEvent.type(await screen.findByLabelText('Note'), 'Second.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await view.rerender({ task: reviewQueueItem({ taskId: '_review' }), headSha: 'c'.repeat(40) });

			expect(await screen.findByRole('alert')).toHaveTextContent('Your notes are');
		});
	});

	describe('the task it is annotating', () => {
		it('accepts a task detail as well as a queue row', async () => {
			const { sent } = stubReject(() => json({ traceId: 't', taskId: 'task-01' }));
			const view = render(CommentThreads, {
				task: taskDetail({ taskId: 'task-01', status: 'waiting_review' }),
				headSha: HEAD
			});
			view.component.anchorOn(anchor());
			await userEvent.type(await screen.findByLabelText('Note'), 'Name this constant.');
			await userEvent.click(screen.getByRole('button', { name: 'Add draft' }));

			await userEvent.click(screen.getByRole('button', { name: 'Request changes' }));

			// The panel is mounted from a deep link as well as from the queue, and the
			// request is addressed by both ids, so the task's own identity is the only
			// thing that can get this right.
			await waitFor(() => expect(sent).toHaveLength(1));
			expect(vi.mocked(fetch).mock.calls[0][0]).toBe(
				'/api/traces/trace-01a0bd6963faa14f/tasks/task-01/reject'
			);
		});

		it('defaults a rework status it was not told, because an unnamed one is still running', () => {
			render(CommentThreads, { task: reviewQueueItem({ taskId: '_review', reworkTaskId: 'task-09' }) });

			expect(screen.getByText(/Rework task-09 is in flight/)).toBeInTheDocument();
		});
	});
});
