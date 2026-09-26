import { describe, expect, it, vi } from 'vitest';
import { createGitStore } from './git.svelte';
import { gitBranch, gitView } from '../../tests/fixtures';
import type { GitActionResult, GitView } from '$lib/api/types';

const ok: GitActionResult = { ok: true, message: 'done' };

/** A store that answers every mutation with `result` and never polls. */
function harness(overrides: Partial<Parameters<typeof createGitStore>[0]> = {}, view?: GitView) {
	const loadGit = vi.fn(async () => view ?? gitView());
	const options = {
		loadGit,
		fetchOrigin: vi.fn(async () => ok),
		push: vi.fn(async () => ok),
		pull: vi.fn(async () => ok),
		pruneWorktrees: vi.fn(async () => ok),
		deleteBranches: vi.fn(async () => ok),
		pollIntervalMs: 0,
		...overrides
	};
	return { store: createGitStore(options), ...options };
}

describe('createGitStore', () => {
	it('skeletons until the first payload and keeps the view after a later failure', async () => {
		const loadGit = vi
			.fn<() => Promise<GitView>>()
			.mockResolvedValueOnce(gitView())
			.mockRejectedValueOnce(new Error('git: not a repository'));
		const store = createGitStore({ loadGit, pollIntervalMs: 0 });

		expect(store.showSkeletons).toBe(true);
		await store.refresh();
		expect(store.showSkeletons).toBe(false);

		await store.refresh();

		expect(store.lastError).toBe('git: not a repository');
		expect(store.git).not.toBeNull();
	});

	it('reads what the page needs off the payload instead of re-deriving it', async () => {
		const { store } = harness();
		await store.refresh();

		expect(store.hasOrigin).toBe(true);
		expect(store.unpublished).toHaveLength(2);
		expect(store.leftoverNames).toEqual(['paseka/trace-019f76d17ca323c8']);
	});

	it('reports no origin rather than offering actions with nothing behind them', async () => {
		const { store } = harness({}, gitView({ originUrl: undefined, ahead: undefined, behind: undefined }));
		await store.refresh();

		expect(store.hasOrigin).toBe(false);
	});

	it('re-reads the clone after a mutation so the view is never the state before it', async () => {
		const { store, loadGit, fetchOrigin } = harness();
		await store.refresh();

		const outcome = await store.run('fetch');

		expect(outcome).toEqual({ status: 'done', result: ok });
		expect(fetchOrigin).toHaveBeenCalledTimes(1);
		expect(loadGit).toHaveBeenCalledTimes(2);
	});

	it('passes the push-hooks flag through and ignores it for the other actions', async () => {
		const { store, push, pull } = harness();
		await store.refresh();

		await store.run('push', true);
		await store.run('pull', true);

		expect(push).toHaveBeenCalledWith(true);
		expect(pull).toHaveBeenCalledWith();
	});

	it('deletes exactly the leftover branches, in one request', async () => {
		const { store, deleteBranches } = harness(
			{},
			gitView({
				branches: [
					gitBranch({ name: 'main' }),
					gitBranch({ name: 'paseka/gone', merged: true, leftover: true }),
					gitBranch({ name: 'feature/kept', worktreePath: '/colony/wt', merged: true })
				]
			})
		);
		await store.refresh();

		await store.run('delete');

		expect(deleteBranches).toHaveBeenCalledWith(['paseka/gone']);
	});

	it('keeps a failure as a reason rather than throwing, and clears it on the next attempt', async () => {
		const pull = vi
			.fn<() => Promise<GitActionResult>>()
			.mockRejectedValueOnce(new Error('live bee is using the colony root checkout; refuse pull'))
			.mockResolvedValueOnce(ok);
		const { store } = harness({ pull });
		await store.refresh();

		const refused = await store.run('pull');

		expect(refused).toEqual({
			status: 'failed',
			message: 'live bee is using the colony root checkout; refuse pull'
		});
		expect(store.actionError).toBe('live bee is using the colony root checkout; refuse pull');

		await store.run('pull');
		expect(store.actionError).toBe('');
	});

	it('refuses a second mutation while one is in flight', async () => {
		let release: (() => void) | undefined;
		const gate = new Promise<void>((resolve) => (release = resolve));
		const pruneWorktrees = vi.fn(async () => {
			await gate;
			return ok;
		});
		const { store } = harness({ pruneWorktrees });
		await store.refresh();

		const first = store.run('prune');
		const second = await store.run('push');
		expect(second).toEqual({ status: 'busy' });
		expect(store.busy).toBe(true);
		release?.();
		await first;

		expect(pruneWorktrees).toHaveBeenCalledTimes(1);
		expect(store.busy).toBe(false);
		expect(store.pending).toBeNull();
	});

	it('polls on an interval and stops on unmount, so an idle console shells out to git not at all', async () => {
		vi.useFakeTimers();
		try {
			const loadGit = vi.fn(async () => gitView());
			const store = createGitStore({ loadGit, pollIntervalMs: 1000 });

			store.start();
			store.start();
			await vi.advanceTimersByTimeAsync(1000);
			expect(loadGit).toHaveBeenCalledTimes(2);

			store.stop();
			await vi.advanceTimersByTimeAsync(5000);
			expect(loadGit).toHaveBeenCalledTimes(2);
		} finally {
			vi.useRealTimers();
		}
	});
});
