import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Git from './+page.svelte';
import { createGitStore, type GitStore } from '$lib/stores/git.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { gitBranch, gitView, gitWorktree } from '../../tests/fixtures';
import type { GitActionResult, GitView } from '$lib/api/types';

const ok: GitActionResult = { ok: true, message: 'done' };

interface Harness {
	store: GitStore;
	toasts: ReturnType<typeof createToastStore>;
	fetchOrigin: ReturnType<typeof vi.fn>;
	push: ReturnType<typeof vi.fn>;
	pull: ReturnType<typeof vi.fn>;
	pruneWorktrees: ReturnType<typeof vi.fn>;
	deleteBranches: ReturnType<typeof vi.fn>;
}

function harness(
	view: GitView = gitView(),
	overrides: Partial<Parameters<typeof createGitStore>[0]> = {}
): Harness {
	const fetchOrigin = vi.fn(async () => ok);
	const push = vi.fn(async () => ok);
	const pull = vi.fn(async () => ok);
	const pruneWorktrees = vi.fn(async () => ok);
	const deleteBranches = vi.fn(async () => ok);
	const store = createGitStore({
		loadGit: async () => view,
		fetchOrigin,
		push,
		pull,
		pruneWorktrees,
		deleteBranches,
		pollIntervalMs: 0,
		...overrides
	});
	return { store, toasts: createToastStore(0), fetchOrigin, push, pull, pruneWorktrees, deleteBranches };
}

/** The single toast currently on screen, tone included. */
function lastToast(toasts: ReturnType<typeof createToastStore>) {
	const item = toasts.items[toasts.items.length - 1];
	return { tone: item?.tone, message: item?.message };
}

describe('git route', () => {
	it('splits the clone from the origin and leaves the sync numbers inside one word', async () => {
		const { store, toasts } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('heading', { name: 'Worktrees' })).toBeInTheDocument());

		const clone = screen.getByLabelText('Colony root');
		const origin = screen.getByLabelText('Origin');
		expect(within(clone).getByText('Branch')).toBeInTheDocument();
		expect(within(clone).getByText('Working tree')).toBeInTheDocument();
		expect(within(origin).getByText('Sync')).toBeInTheDocument();
		expect(within(origin).getByText('↑3')).toBeInTheDocument();
		// The divergence is stated once, not as an Ahead row beside a Sync row.
		expect(within(origin).queryByText('Ahead')).not.toBeInTheDocument();
	});

	it('offers the three sync actions compactly, with Push primary only when there is work', async () => {
		const { store, toasts } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Fetch' })).toBeInTheDocument());

		const push = screen.getByRole('button', { name: 'Push' });
		expect(push).toHaveClass('btn-primary');
		for (const name of ['Fetch', 'Pull']) {
			expect(screen.getByRole('button', { name })).not.toHaveClass('btn-primary');
		}

		const clean = harness(gitView({ unpublished: [] }));
		render(Git, { store: clean.store, toasts: clean.toasts });
		await waitFor(() => expect(screen.getAllByRole('button', { name: 'Push' })[1]).toBeInTheDocument());
		expect(screen.getAllByRole('button', { name: 'Push' })[1]).not.toHaveClass('btn-primary');
	});

	it('runs a sync action on one click and reports it, with no confirmation in the way', async () => {
		const user = userEvent.setup();
		const { store, toasts, fetchOrigin } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Fetch' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Fetch' }));

		await waitFor(() => expect(fetchOrigin).toHaveBeenCalledTimes(1));
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
		await waitFor(() => expect(lastToast(toasts)).toEqual({ tone: 'success', message: 'done' }));
	});

	it('sends the hooks toggle with a push and leaves it out of the other actions', async () => {
		const user = userEvent.setup();
		const { store, toasts, push, pull } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Push' })).toBeInTheDocument());

		await user.click(screen.getByRole('checkbox', { name: 'Run git hooks on push' }));
		await user.click(screen.getByRole('button', { name: 'Push' }));
		await waitFor(() => expect(push).toHaveBeenCalledWith(true));

		await user.click(screen.getByRole('button', { name: 'Pull' }));
		await waitFor(() => expect(pull).toHaveBeenCalledWith());
	});

	it('disables the sync actions without an origin and says why on the page', async () => {
		const { store, toasts } = harness(
			gitView({ originUrl: undefined, ahead: undefined, behind: undefined })
		);
		render(Git, { store, toasts });

		await waitFor(() => expect(screen.getByRole('button', { name: 'Fetch' })).toBeDisabled());
		expect(screen.getByRole('button', { name: 'Push' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Pull' })).toBeDisabled();
		// A disabled control cannot carry a tooltip, so the reason is stated in the copy.
		expect(
			screen.getByText(/No origin remote, so there is nothing to fetch, push, or pull/)
		).toBeInTheDocument();
	});

	it('links each worktree to its trail, and leaves an unregistered one as plain text', async () => {
		const { store, toasts } = harness(
			gitView({
				worktrees: [
					gitWorktree({ traceId: 'trace-01', branch: 'paseka/trace-01' }),
					gitWorktree({
						traceId: undefined,
						branch: 'paseka/orphan',
						path: '/colony/.paseka/worktrees/orphan'
					})
				]
			})
		);
		render(Git, { store, toasts });

		await waitFor(() =>
			expect(screen.getByRole('link', { name: 'trace-01' })).toHaveAttribute(
				'href',
				'/next/traces/trace-01'
			)
		);
		const orphan = screen.getByText('paseka/orphan').closest('tr');
		const cells = within(orphan as HTMLElement).getAllByRole('cell');
		expect(within(cells[1] as HTMLElement).queryByRole('link')).not.toBeInTheDocument();
	});

	it('badges a worktree dirty or clean and links an open pull request', async () => {
		const { store, toasts } = harness(
			gitView({
				worktrees: [
					gitWorktree({ traceId: 'trace-01', branch: 'paseka/trace-01', dirty: true }),
					gitWorktree({
						traceId: 'trace-02',
						branch: 'paseka/trace-02',
						path: '/colony/.paseka/worktrees/trace-02',
						prUrl: 'https://example.test/7'
					})
				]
			})
		);
		render(Git, { store, toasts });

		await waitFor(() => expect(screen.getByText('dirty')).toBeInTheDocument());
		expect(screen.getByText('clean')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'open' })).toHaveAttribute('href', 'https://example.test/7');
		// A worktree with no pull request leaves the cell empty rather than a dead `open`.
		const orphan = screen.getByText('paseka/trace-01').closest('tr');
		const cells = within(orphan as HTMLElement).getAllByRole('cell');
		expect(cells[3]).toBeEmptyDOMElement();
	});

	it('names one branch state and keeps the rest as quiet flags', async () => {
		const { store, toasts } = harness(
			gitView({
				branches: [
					gitBranch({ name: 'main' }),
					gitBranch({
						name: 'paseka/gone',
						current: false,
						default: false,
						merged: true,
						leftover: true
					}),
					gitBranch({ name: 'paseka/held', current: false, default: false, worktreePath: '/colony/wt' })
				]
			})
		);
		render(Git, { store, toasts });

		await waitFor(() => expect(screen.getByText('current')).toBeInTheDocument());
		expect(screen.getByText('leftover')).toBeInTheDocument();
		// A branch that is neither current, merged, nor leftover says nothing at all.
		expect(screen.queryByText('merged')).not.toBeInTheDocument();
		expect(screen.getByText('default')).toBeInTheDocument();
		expect(screen.getByText('worktree')).toBeInTheDocument();
	});

	it('filters branches by name, subject, or the trail behind them', async () => {
		const user = userEvent.setup();
		const { store, toasts } = harness(
			gitView({
				branches: [
					gitBranch({ name: 'paseka/gone', current: false, default: false, subject: 'Drop the old seam' }),
					gitBranch({
						name: 'paseka/held',
						current: false,
						default: false,
						traceId: 'trace-needle',
						subject: 'Keep the new one'
					})
				]
			})
		);
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByText('paseka/gone')).toBeInTheDocument());

		await user.type(screen.getByLabelText('Filter branches'), 'trace-needle');

		expect(screen.getByText('paseka/held')).toBeInTheDocument();
		expect(screen.queryByText('paseka/gone')).not.toBeInTheDocument();
	});

	it('confirms the leftover sweep, names every branch, and deletes them in one request', async () => {
		const user = userEvent.setup();
		const { store, toasts, deleteBranches } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: /Delete 1 leftover/ })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: /Delete 1 leftover/ }));

		const dialog = await screen.findByRole('dialog', { name: 'Delete merged leftover branches?' });
		expect(within(dialog).getByText('paseka/trace-019f76d17ca323c8')).toBeInTheDocument();
		await user.click(within(dialog).getByRole('button', { name: 'Delete leftovers' }));

		await waitFor(() => expect(deleteBranches).toHaveBeenCalledWith(['paseka/trace-019f76d17ca323c8']));
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
	});

	it('offers no sweep when nothing is left over', async () => {
		const { store, toasts } = harness(gitView({ branches: [gitBranch()] }));
		render(Git, { store, toasts });

		await waitFor(() => expect(screen.getByRole('button', { name: 'No merged leftovers' })).toBeDisabled());
	});

	it('confirms the prune and keeps the dialog open with the reason when it fails', async () => {
		const user = userEvent.setup();
		const pruneWorktrees = vi
			.fn<() => Promise<GitActionResult>>()
			.mockRejectedValue(new Error('worktree prune failed: not a git repository'));
		const { store, toasts } = harness(gitView(), { pruneWorktrees });
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Prune orphans' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Prune orphans' }));
		const dialog = await screen.findByRole('dialog', { name: 'Prune orphan worktrees?' });
		await user.click(within(dialog).getByRole('button', { name: 'Prune orphans' }));

		await waitFor(() =>
			expect(
				within(screen.getByRole('dialog')).getByRole('alert')
			).toHaveTextContent('worktree prune failed: not a git repository')
		);
		expect(pruneWorktrees).toHaveBeenCalledTimes(1);
		// The operator can retry from where they are, and nothing was reported twice.
		expect(toasts.items).toHaveLength(0);
	});

	it('cancels a confirmation without touching the clone', async () => {
		const user = userEvent.setup();
		const { store, toasts, deleteBranches } = harness();
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: /Delete 1 leftover/ })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: /Delete 1 leftover/ }));
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(deleteBranches).not.toHaveBeenCalled();
	});

	it('warns rather than claims success when only some branches were deleted', async () => {
		const user = userEvent.setup();
		const deleteBranches = vi.fn(async () => ({
			ok: false,
			results: [
				{ name: 'paseka/gone', ok: true },
				{ name: 'main', ok: false, error: 'branch is checked out' }
			]
		}));
		const { store, toasts } = harness(
			gitView({
				branches: [
					gitBranch({ name: 'main' }),
					gitBranch({
						name: 'paseka/gone',
						current: false,
						default: false,
						merged: true,
						leftover: true
					})
				]
			}),
			{ deleteBranches }
		);
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: /Delete 1 leftover/ })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: /Delete 1 leftover/ }));
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Delete leftovers' }));

		await waitFor(() =>
			expect(lastToast(toasts)).toEqual({
				tone: 'warning',
				message: 'main: branch is checked out'
			})
		);
	});

	it('reports a failed header action as an error toast, since it has no dialog to stay in', async () => {
		const user = userEvent.setup();
		const pull = vi
			.fn<() => Promise<GitActionResult>>()
			.mockRejectedValue(new Error('live bee is using the colony root checkout; refuse pull'));
		const { store, toasts } = harness(gitView(), { pull });
		render(Git, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Pull' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Pull' }));

		await waitFor(() =>
			expect(lastToast(toasts)).toEqual({
				tone: 'error',
				message: 'live bee is using the colony root checkout; refuse pull'
			})
		);
	});

	it('shows the unpublished commits, and no section at all when there is nothing to push', async () => {
		const { store, toasts } = harness();
		const { unmount } = render(Git, { store, toasts });

		await waitFor(() =>
			expect(screen.getByRole('heading', { name: /Unpublished commits/ })).toBeInTheDocument()
		);
		expect(screen.getByText('2 to push')).toBeInTheDocument();
		expect(
			screen.getByText('feat(console): migrate the traces list and trail detail to /next')
		).toBeInTheDocument();
		unmount();

		const clean = harness(gitView({ unpublished: [] }));
		render(Git, { store: clean.store, toasts: clean.toasts });
		await waitFor(() => expect(screen.getByRole('heading', { name: 'Worktrees' })).toBeInTheDocument());
		expect(screen.queryByRole('heading', { name: /Unpublished commits/ })).not.toBeInTheDocument();
	});

	it('skeletons while the first read is in flight', async () => {
		let release: ((view: GitView) => void) | undefined;
		const gate = new Promise<GitView>((resolve) => (release = resolve));
		const { store, toasts } = harness(gitView(), { loadGit: () => gate });
		const { container, unmount } = render(Git, { store, toasts });

		await waitFor(() => expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		unmount();
		release?.(gitView());
	});

	it('surfaces a read failure as an alert and drops the skeletons with it', async () => {
		const { store, toasts } = harness(gitView(), {
			loadGit: async () => {
				throw new Error('git: not a git repository');
			}
		});
		const { container } = render(Git, { store, toasts });

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('not a git repository'));
		expect(container.querySelectorAll('.skeleton').length).toBe(0);
		expect(screen.queryByRole('table')).not.toBeInTheDocument();
	});
});
