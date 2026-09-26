import {
	getGit as requestGit,
	gitDeleteBranches as requestDeleteBranches,
	gitFetch as requestFetch,
	gitPruneWorktrees as requestPrune,
	gitPull as requestPull,
	gitPush as requestPush
} from '$lib/api/client';
import type { GitActionResult, GitCommit, GitView } from '$lib/api/types';
import { gitLeftoverNames, type GitAction } from '$lib/format';

/** What a mutation answered with, so a caller can tell a refusal from a failure. */
export type GitRunOutcome =
	| { status: 'done'; result: GitActionResult }
	| { status: 'failed'; message: string }
	| { status: 'busy' };

interface GitStoreOptions {
	loadGit?: () => Promise<GitView>;
	fetchOrigin?: () => Promise<GitActionResult>;
	push?: (runHooks: boolean) => Promise<GitActionResult>;
	pull?: () => Promise<GitActionResult>;
	pruneWorktrees?: () => Promise<GitActionResult>;
	deleteBranches?: (names: string[]) => Promise<GitActionResult>;
	/** The clone is a shell-out per read, so this is the console's slowest poll. */
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony clone behind `/next/git`. It is a route-scoped poller for its own
 * endpoint — the topbar's git plaque rides the chrome stream, not this — and it
 * starts on mount and stops on unmount, so an idle console runs no `git` shell
 * out at all.
 *
 * A mutation re-reads the clone afterwards, so the view never shows the state
 * before the action that just ran. Exactly one mutation is in flight at a time:
 * a push during a prune would race on the same refs.
 */
export function createGitStore(options: GitStoreOptions = {}) {
	const readGit = options.loadGit ?? requestGit;
	const fetchOrigin = options.fetchOrigin ?? requestFetch;
	const pushOrigin = options.push ?? requestPush;
	const pullOrigin = options.pull ?? requestPull;
	const pruneWorktrees = options.pruneWorktrees ?? requestPrune;
	const deleteBranches = options.deleteBranches ?? requestDeleteBranches;
	const pollIntervalMs = options.pollIntervalMs ?? 15000;

	let view = $state<GitView | null>(null);
	let loading = $state(true);
	let error = $state('');
	let actionError = $state('');
	let pending = $state<GitAction | null>(null);
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	async function refresh(): Promise<void> {
		try {
			view = await readGit();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	function mutate(action: GitAction, runHooks: boolean): Promise<GitActionResult> {
		switch (action) {
			case 'fetch':
				return fetchOrigin();
			case 'push':
				return pushOrigin(runHooks);
			case 'pull':
				return pullOrigin();
			case 'prune':
				return pruneWorktrees();
			default:
				return deleteBranches(gitLeftoverNames(view?.branches));
		}
	}

	async function run(action: GitAction, runHooks = false): Promise<GitRunOutcome> {
		if (pending !== null) return { status: 'busy' };
		pending = action;
		actionError = '';
		try {
			const result = await mutate(action, runHooks);
			await refresh();
			return { status: 'done', result };
		} catch (cause) {
			actionError = errorMessage(cause);
			return { status: 'failed', message: actionError };
		} finally {
			pending = null;
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
		get git(): GitView | null {
			return view;
		},
		/** Skeletons only before the first payload; a later failure keeps the view and shows the alert. */
		get showSkeletons(): boolean {
			return loading && view === null;
		},
		get lastError(): string {
			return error;
		},
		/** The reason the last mutation failed, kept for a dialog that stays open on it. */
		get actionError(): string {
			return actionError;
		},
		/** Which mutation is in flight, or `null`; a button names its own verb while it waits. */
		get pending(): GitAction | null {
			return pending;
		},
		get busy(): boolean {
			return pending !== null;
		},
		/** Without a remote there is nothing to fetch, push, or pull. */
		get hasOrigin(): boolean {
			return Boolean(view?.originUrl);
		},
		/** Never `undefined`: the server omits the list when it could not compare against origin. */
		get unpublished(): GitCommit[] {
			return view?.unpublished ?? [];
		},
		get leftoverNames(): string[] {
			return gitLeftoverNames(view?.branches);
		},
		refresh,
		run,
		start,
		stop
	};
}

export type GitStore = ReturnType<typeof createGitStore>;
