import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Worktrees from './+page.svelte';
import { createGitStore, type GitStore } from '$lib/stores/git.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';
import { gitView, gitWorktree } from '../../tests/fixtures';
import type { GitActionResult, GitView } from '$lib/api/types';

const ok: GitActionResult = { ok: true, message: 'done' };

interface Harness {
	store: GitStore;
	toasts: ReturnType<typeof createToastStore>;
	pruneWorktrees: ReturnType<typeof vi.fn>;
	loadGit: ReturnType<typeof vi.fn>;
}

function harness(
	view: GitView = gitView(),
	overrides: Partial<Parameters<typeof createGitStore>[0]> = {}
): Harness {
	const defaults = {
		loadGit: vi.fn(async () => view),
		fetchOrigin: vi.fn(async () => ok),
		push: vi.fn(async () => ok),
		pull: vi.fn(async () => ok),
		pruneWorktrees: vi.fn(async () => ok),
		deleteBranches: vi.fn(async () => ok)
	};
	const store = createGitStore({ ...defaults, pollIntervalMs: 0, ...overrides });
	return {
		store,
		toasts: createToastStore(0),
		// The handles are the ones the store actually holds, so an override is assertable.
		loadGit: (overrides.loadGit ?? defaults.loadGit) as Harness['loadGit'],
		pruneWorktrees: (overrides.pruneWorktrees ?? defaults.pruneWorktrees) as Harness['pruneWorktrees']
	};
}

/** The single toast currently on screen, tone included. */
function lastToast(toasts: ReturnType<typeof createToastStore>) {
	const item = toasts.items[toasts.items.length - 1];
	return { tone: item?.tone, message: item?.message };
}

/** The cells of the row holding `text`, so a column can be asserted by position. */
function cellsOf(text: string): HTMLElement[] {
	const row = screen.getByText(text).closest('tr');
	return within(row as HTMLElement).getAllByRole('cell') as HTMLElement[];
}

describe('worktrees route', () => {
	it('reads the worktree list off the one git endpoint, without a second read per route', async () => {
		const { store, toasts, loadGit } = harness();
		render(Worktrees, { store, toasts });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		expect(loadGit).toHaveBeenCalledTimes(1);
		// The store reads `GET /api/git` once on mount and, with no timer configured, never again.
		expect(store.git?.worktrees).toHaveLength(2);
	});

	it('names what the colony is holding, in a sentence rather than a plural of nothing', async () => {
		const { store, toasts } = harness();
		render(Worktrees, { store, toasts });
		await waitFor(() => expect(screen.getByText('2 isolated worktrees, one per trail.')).toBeInTheDocument());

		const one = harness(gitView({ worktrees: [gitWorktree()] }));
		const single = render(Worktrees, { store: one.store, toasts: one.toasts });
		await waitFor(() =>
			expect(screen.getByText('One isolated worktree, holding the branch of one trail.')).toBeInTheDocument()
		);
		single.unmount();

		const none = harness(gitView({ worktrees: [] }));
		render(Worktrees, { store: none.store, toasts: none.toasts });
		await waitFor(() =>
			expect(screen.getByText('No isolated worktree exists yet.')).toBeInTheDocument()
		);
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
		render(Worktrees, { store, toasts });

		await waitFor(() =>
			expect(screen.getByRole('link', { name: 'trace-01' })).toHaveAttribute(
				'href',
				'/next/traces/trace-01'
			)
		);
		const cells = cellsOf('paseka/orphan');
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
		render(Worktrees, { store, toasts });

		await waitFor(() => expect(screen.getByText('dirty')).toBeInTheDocument());
		expect(screen.getByText('clean')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'open' })).toHaveAttribute('href', 'https://example.test/7');
		// A worktree with no pull request leaves the cell empty rather than a dead `open`.
		expect(cellsOf('paseka/trace-01')[3]).toBeEmptyDOMElement();
	});

	it('filters by branch, trail, or the path that is not on screen', async () => {
		const user = userEvent.setup();
		const { store, toasts } = harness(
			gitView({
				worktrees: [
					gitWorktree({ branch: 'paseka/one', path: '/colony/.paseka/worktrees/trace-01' }),
					gitWorktree({ branch: 'paseka/two', path: '/colony/.paseka/worktrees/trace-02' })
				]
			})
		);
		render(Worktrees, { store, toasts });
		await waitFor(() => expect(screen.getByText('paseka/one')).toBeInTheDocument());

		// The path is derivable from the trail, so it is searchable but never a column.
		await user.type(screen.getByLabelText('Filter worktrees'), 'trace-02');
		expect(screen.getByText('paseka/two')).toBeInTheDocument();
		expect(screen.queryByText('paseka/one')).not.toBeInTheDocument();
	});

	it('confirms the prune, and says the dialog stays open when it fails', async () => {
		const user = userEvent.setup();
		const pruneWorktrees = vi
			.fn<() => Promise<GitActionResult>>()
			.mockRejectedValue(new Error('worktree prune failed: not a git repository'));
		const { store, toasts } = harness(gitView(), { pruneWorktrees });
		render(Worktrees, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Prune orphans' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Prune orphans' }));
		const dialog = await screen.findByRole('dialog', { name: 'Prune orphan worktrees?' });
		// Branches survive a prune, so the dialog says so before anything is destroyed.
		expect(dialog).toHaveTextContent('Branches are kept');
		await user.click(within(dialog).getByRole('button', { name: 'Prune orphans' }));

		await waitFor(() =>
			expect(within(screen.getByRole('dialog')).getByRole('alert')).toHaveTextContent(
				'worktree prune failed: not a git repository'
			)
		);
		expect(pruneWorktrees).toHaveBeenCalledTimes(1);
		// The operator can retry from where they are, and nothing was reported twice.
		expect(toasts.items).toHaveLength(0);
	});

	it('reports what the prune reconciled, since git prints nothing on a clean sweep', async () => {
		const user = userEvent.setup();
		const { store, toasts, pruneWorktrees } = harness(gitView(), {
			pruneWorktrees: vi.fn(async () => ({
				ok: true,
				message: 'unregistered: trace-01a0bd6963faa14f'
			}))
		});
		render(Worktrees, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Prune orphans' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Prune orphans' }));
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Prune orphans' }));

		await waitFor(() =>
			expect(lastToast(toasts)).toEqual({
				tone: 'success',
				message: 'unregistered: trace-01a0bd6963faa14f'
			})
		);
		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		// The list is re-read after the mutation, so a dropped row needs no manual refresh.
		await waitFor(() => expect(pruneWorktrees).toHaveBeenCalledTimes(1));
		expect(store.git?.worktrees).toHaveLength(2);
	});

	it('cancels the prune without touching the clone', async () => {
		const user = userEvent.setup();
		const { store, toasts, pruneWorktrees } = harness();
		render(Worktrees, { store, toasts });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Prune orphans' })).toBeInTheDocument());

		await user.click(screen.getByRole('button', { name: 'Prune orphans' }));
		const dialog = await screen.findByRole('dialog');
		await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));

		await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
		expect(pruneWorktrees).not.toHaveBeenCalled();
	});

	it('an empty colony says so instead of showing a table of nothing', async () => {
		const { store, toasts } = harness(gitView({ worktrees: [] }));
		render(Worktrees, { store, toasts });

		await waitFor(() =>
			expect(screen.getByText('No colony-managed worktrees.')).toBeInTheDocument()
		);
		expect(screen.queryByRole('link', { name: /\/next\/traces/ })).not.toBeInTheDocument();
	});

	it('skeletons while the first read is in flight, and alerts when it fails', async () => {
		let release: ((view: GitView) => void) | undefined;
		const gate = new Promise<GitView>((resolve) => (release = resolve));
		const slow = harness(gitView(), { loadGit: () => gate });
		const { container, unmount } = render(Worktrees, { store: slow.store, toasts: slow.toasts });

		await waitFor(() => expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0));
		unmount();
		release?.(gitView());

		const failed = harness(gitView(), {
			loadGit: async () => {
				throw new Error('git: not a git repository');
			}
		});
		const broken = render(Worktrees, { store: failed.store, toasts: failed.toasts });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('not a git repository'));
		expect(broken.container.querySelectorAll('.skeleton').length).toBe(0);
		expect(screen.queryByRole('table')).not.toBeInTheDocument();
	});
});
