import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ReviewActions from './ReviewActions.svelte';
import { reviewQueueItem } from '../../tests/fixtures';
import type { ReviewQueueItem, TaskDetail } from '$lib/api/types';

function mount(task: TaskDetail | ReviewQueueItem = reviewQueueItem({ taskId: '_review', isFinal: true })) {
	const onsettled = vi.fn();
	const view = render(ReviewActions, { task, onsettled });
	return { ...view, onsettled };
}

interface Sent {
	url: string;
	body: Record<string, unknown>;
}

/** One stub for both decisions: the panel posts to the task's own path either way. */
function stubActions(
	result: () => Response = () => json({ traceId: 't', taskId: '_review' })
): { sent: Sent[] } {
	const sent: Sent[] = [];
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			sent.push({ url: String(input), body: JSON.parse(String(init?.body ?? '{}')) });
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

describe('ReviewActions', () => {
	describe('the collapsed state', () => {
		it('offers two verbs and no form, because six fields above the fold compete with the diff', () => {
			mount();

			expect(screen.getByRole('button', { name: 'Approve…' })).toHaveAttribute('aria-expanded', 'false');
			expect(screen.getByRole('button', { name: 'Reject…' })).toHaveAttribute('aria-expanded', 'false');
			expect(screen.queryByLabelText('Approval summary')).not.toBeInTheDocument();
			expect(screen.queryByLabelText('Feedback')).not.toBeInTheDocument();
		});

		it('opens one form at a time, and closes the one that was open', async () => {
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			expect(screen.getByRole('button', { name: 'Approve…' })).toHaveAttribute('aria-expanded', 'true');
			expect(screen.getByLabelText('Approval summary')).toBeInTheDocument();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			expect(screen.getByLabelText('Feedback')).toBeInTheDocument();
			// Two forms at once would be two verdicts to read before choosing one.
			expect(screen.queryByLabelText('Approval summary')).not.toBeInTheDocument();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			expect(screen.queryByLabelText('Feedback')).not.toBeInTheDocument();
		});

		it('clears a previous failure when the form is reopened', async () => {
			stubActions(() => new Response('worktree is dirty', { status: 409 }));
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Looks right.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));
			expect(await screen.findByRole('alert')).toHaveTextContent('worktree is dirty');

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));

			// A stale reason attached to a form nobody has filled in yet is a reason to
			// distrust the next one too.
			expect(screen.queryByRole('alert')).not.toBeInTheDocument();
		});
	});

	describe('approving a local merge', () => {
		it('says what approving will do, before the operator commits to it', async () => {
			mount(reviewQueueItem({ taskId: '_review', isFinal: true, delivery: 'local_merge' }));

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));

			expect(
				screen.getByText('Merges this trail’s worktree branch into the default branch.')
			).toBeInTheDocument();
		});

		it('needs a summary, because an approval recorded with nothing said is not a sign-off', async () => {
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			const approve = screen.getByRole('button', { name: 'Approve' });

			expect(approve).toBeDisabled();
			await userEvent.type(screen.getByLabelText('Approval summary'), '   ');
			expect(approve).toBeDisabled();
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
			expect(approve).toBeEnabled();
		});

		it('offers a commit message and sends it only when one was written', async () => {
			const { sent } = stubActions(() => json({ traceId: 't', taskId: '_review' }));
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			// A present-but-empty commit message is not the same as an absent one, where
			// absent means "use the trail summary".
			expect(sent[0].body).toEqual({ summary: 'Checked the merge body.' });
			expect(sent[0].url).toBe('/api/traces/trace-01a0bd6963faa14f/tasks/_review/approve');
		});

		it('sends a commit message the reviewer wrote', async () => {
			const { sent } = stubActions();
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
			await userEvent.type(screen.getByLabelText('Commit message'), 'Add the export --format flag');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0].body).toEqual({
				summary: 'Checked the merge body.',
				mergeMessage: 'Add the export --format flag'
			});
		});

		it('reports the server\'s own message when it sent one', async () => {
			stubActions(() => json({ traceId: 't', taskId: '_review', message: 'Merged into main.' }));
			const { onsettled } = mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked it.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			expect(onsettled).toHaveBeenCalledWith({ tone: 'success', message: 'Merged into main.' });
		});

		it('names the approval it made when the server said nothing at all', async () => {
			stubActions();
			const { onsettled } = mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked it.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			expect(onsettled).toHaveBeenCalledWith({ tone: 'success', message: 'Approved _review' });
		});
	});

	describe('approving a delivery that is a pull request', () => {
		const pullRequest = reviewQueueItem({
			taskId: '_review',
			isFinal: true,
			delivery: 'pull_request',
			prTitle: 'Add paseka export --format',
			prBody: 'Adds a --format flag to `paseka export`.'
		});

		it('replaces the commit message with the PR fields, which are the delivery\'s own', async () => {
			mount(pullRequest);

			await userEvent.click(screen.getByRole('button', { name: 'Open PR…' }));

			// A merge and a PR are not the same action, and a commit message has nowhere
			// to go on a colony that publishes a pull request instead.
			expect(screen.queryByLabelText('Commit message')).not.toBeInTheDocument();
			expect(screen.getByLabelText('PR title')).toBeInTheDocument();
			expect(screen.getByLabelText('PR body')).toBeInTheDocument();
		});

		it('fills the PR copy from the server, and says the branch is pushed first', async () => {
			mount(pullRequest);

			await userEvent.click(screen.getByRole('button', { name: 'Open PR…' }));

			expect(screen.getByLabelText('PR title')).toHaveValue('Add paseka export --format');
			expect(screen.getByLabelText('PR body')).toHaveValue('Adds a --format flag to `paseka export`.');
			expect(
				screen.getByText('Pushes the worktree branch and opens a pull request for it.')
			).toBeInTheDocument();
		});

		it('pre-fills only an empty field, so a half-written title survives a re-read', async () => {
			const view = mount(pullRequest);

			await userEvent.click(screen.getByRole('button', { name: 'Open PR…' }));
			await userEvent.type(screen.getByLabelText('PR title'), ' — mine');

			// The queue poll re-renders this form on every tick, and a pre-fill that ran
			// on each one would discard a title the operator was halfway through.
			await view.rerender({ task: { ...pullRequest, prTitle: 'A newer title from the server' } });

			expect(screen.getByLabelText('PR title')).toHaveValue('Add paseka export --format — mine');
		});

		it('sends the delivery fields with the two flags the server acts on', async () => {
			const { sent } = stubActions(() => json({ traceId: 't', taskId: '_review', prUrl: 'https://example.test/pr/1' }));
			mount(pullRequest);

			await userEvent.click(screen.getByRole('button', { name: 'Open PR…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Looks right.');
			await userEvent.click(screen.getByLabelText('Open as draft'));
			await userEvent.click(screen.getByLabelText('Run git hooks on push'));
			await userEvent.click(screen.getByRole('button', { name: 'Open PR' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0].body).toEqual({
				summary: 'Looks right.',
				prTitle: 'Add paseka export --format',
				prBody: 'Adds a --format flag to `paseka export`.',
				draft: true,
				runHooks: true
			});
		});

		it('says it will update a pull request that is already open, rather than open a second one', async () => {
			mount({
				...pullRequest,
				pullRequest: { url: 'https://example.test/pr/1', number: 1, state: 'open' }
			});

			expect(await screen.findByRole('button', { name: 'Update PR…' })).toBeInTheDocument();
			await userEvent.click(screen.getByRole('button', { name: 'Update PR…' }));
			expect(
				screen.getByText('Pushes the worktree branch and updates its pull request.')
			).toBeInTheDocument();
		});

		it('hands back the pull request the server opened when it said nothing itself', async () => {
			stubActions(() => json({ traceId: 't', taskId: '_review', prUrl: 'https://example.test/pr/1' }));
			const { onsettled } = mount(pullRequest);

			await userEvent.click(screen.getByRole('button', { name: 'Open PR…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Looks right.');
			await userEvent.click(screen.getByRole('button', { name: 'Open PR' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			// Without the link the operator has no way to reach what was just approved.
			expect(onsettled).toHaveBeenCalledWith({
				tone: 'success',
				message: 'Open PR: https://example.test/pr/1'
			});
		});
	});

	describe('a review that is not a final gate', () => {
		const midTrail = reviewQueueItem({ taskId: '002-merge-body-compose', isFinal: false });

		it('records a sign-off rather than promising a merge', async () => {
			mount(midTrail);

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));

			expect(screen.getByText('Records your sign-off. The bee’s work counts as accepted.')).toBeInTheDocument();
			// There is no commit to write and nothing to publish, so both fields would
			// be controls whose endpoints do not exist.
			expect(screen.queryByLabelText('Commit message')).not.toBeInTheDocument();
			expect(screen.queryByLabelText('PR title')).not.toBeInTheDocument();
		});

		it('sends the summary alone', async () => {
			const { sent } = stubActions();
			mount(midTrail);

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'The body reads well.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0].body).toEqual({ summary: 'The body reads well.' });
		});

		it('carries no delivery fields at all, even on a colony that delivers as a pull request', async () => {
			// Both sets of delivery fields hang off the final gate, because a mid-trail
			// review merges nothing and publishes nothing. Gating them on delivery alone
			// would put a commit message and a PR title on a review that has neither.
			mount({ ...midTrail, delivery: 'pull_request' });

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));

			expect(screen.queryByLabelText('Commit message')).not.toBeInTheDocument();
			expect(screen.queryByLabelText('PR title')).not.toBeInTheDocument();
		});
	});

	describe('rejecting', () => {
		it('is one box, and says the feedback does not start a rework task', async () => {
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));

			// Request changes, which does start a rework task and carries an annotated
			// packet, lives with the diff — not duplicated here.
			expect(screen.getByText(/It does not start a rework task/)).toBeInTheDocument();
			expect(screen.queryByLabelText('PR body')).not.toBeInTheDocument();
		});

		it('needs feedback, because a rejection with nothing said is a shrug', async () => {
			stubActions();
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			expect(screen.getByRole('button', { name: 'Reject' })).toBeDisabled();

			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');

			expect(screen.getByRole('button', { name: 'Reject' })).toBeEnabled();
		});

		it('publishes the feedback alone, to the task\'s own path', async () => {
			const { sent } = stubActions();
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');
			await userEvent.click(screen.getByRole('button', { name: 'Reject' }));

			await waitFor(() => expect(sent).toHaveLength(1));
			expect(sent[0].url).toBe('/api/traces/trace-01a0bd6963faa14f/tasks/_review/reject');
			// No `comments` and no `headSha`: this path does not annotate a diff.
			expect(sent[0].body).toEqual({ feedback: 'Guard the seam instead.' });
		});

		it('reports the rework task the server created', async () => {
			stubActions(() => json({ traceId: 't', taskId: '_review', reworkTaskId: 'task-09' }));
			const { onsettled } = mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');
			await userEvent.click(screen.getByRole('button', { name: 'Reject' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			expect(onsettled).toHaveBeenCalledWith({ tone: 'warning', message: 'Rework task task-09 created' });
		});

		it('names the rejection it made when the server created no rework', async () => {
			stubActions(() => json({ traceId: 't', taskId: '_review' }));
			const { onsettled } = mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');
			await userEvent.click(screen.getByRole('button', { name: 'Reject' }));

			await waitFor(() => expect(onsettled).toHaveBeenCalled());
			expect(onsettled).toHaveBeenCalledWith({ tone: 'warning', message: 'Rejected _review' });
		});
	});

	describe('when a decision does not land', () => {
		it('keeps the approval open with the reason, and the half-written summary with it', async () => {
			stubActions(() => new Response('worktree branch not found', { status: 409 }));
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			// The form is where the reason belongs: there is no toast region on the task
			// page, and closing would throw away the summary.
			const alert = await screen.findByRole('alert');
			expect(alert).toHaveTextContent('worktree branch not found');
			expect(alert.closest('form')).toContainElement(alert);
			// The summary survives too, so a retry is a click rather than a retype.
			expect(screen.getByLabelText('Approval summary')).toHaveValue('Checked the merge body.');
			expect(screen.getByRole('button', { name: 'Approve' })).toBeEnabled();
		});

		it('keeps the rejection open with the reason too', async () => {
			stubActions(() => new Response('task is not waiting on a human', { status: 409 }));
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');
			await userEvent.click(screen.getByRole('button', { name: 'Reject' }));

			expect(await screen.findByRole('alert')).toHaveTextContent('task is not waiting on a human');
			expect(screen.getByRole('button', { name: 'Reject' })).toBeEnabled();
		});
	});

	describe('after a decision lands', () => {
		it('collapses the form and empties it, so the next decision starts blank', async () => {
			stubActions();
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			await userEvent.type(screen.getByLabelText('Approval summary'), 'Checked the merge body.');
			await userEvent.type(screen.getByLabelText('Commit message'), 'Add the flag');
			await userEvent.click(screen.getByRole('button', { name: 'Approve' }));

			await waitFor(() => expect(screen.queryByLabelText('Approval summary')).not.toBeInTheDocument());
			expect(screen.getByRole('button', { name: 'Approve…' })).toHaveAttribute('aria-expanded', 'false');

			// Reopened empty, because a stale commit message would be sent silently.
			await userEvent.click(screen.getByRole('button', { name: 'Approve…' }));
			expect(screen.getByLabelText('Approval summary')).toHaveValue('');
			expect(screen.getByLabelText('Commit message')).toHaveValue('');
		});

		it('closes the form on Cancel without deciding anything', async () => {
			const { sent } = stubActions();
			mount();

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			await userEvent.type(screen.getByLabelText('Feedback'), 'Guard the seam instead.');
			await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

			expect(screen.queryByLabelText('Feedback')).not.toBeInTheDocument();
			expect(sent).toHaveLength(0);

			await userEvent.click(screen.getByRole('button', { name: 'Reject…' }));
			expect(screen.getByLabelText('Feedback')).toHaveValue('');
		});
	});
});
