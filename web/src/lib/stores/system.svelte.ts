import { getSystem as requestSystem } from '$lib/api/client';
import type { SystemProcess, SystemView } from '$lib/api/types';

interface SystemStoreOptions {
	loadSystem?: () => Promise<SystemView>;
	/**
	 * The process list makes this the console's busiest poll: it reads three
	 * files for every pid on the box. 5s keeps load legible while an operator
	 * watches a test suite without re-walking `/proc` 20 times a minute.
	 */
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The host snapshot behind `/next/system`. It is a route-scoped poller for its
 * own endpoint and starts on mount, stops on unmount, so an idle console reads
 * no `/proc` at all — the topbar's Host plaque rides the chrome stream, which
 * carries the same struct without the process list.
 *
 * A failure keeps the previous snapshot rather than blanking the page: the
 * numbers an operator is reading are still the last real ones, and the alert
 * says they stopped moving. The server's own partial-failure string is kept
 * apart from that, because it means "this snapshot is incomplete", not "the
 * last read failed".
 */
export function createSystemStore(options: SystemStoreOptions = {}) {
	const readSystem = options.loadSystem ?? requestSystem;
	const pollIntervalMs = options.pollIntervalMs ?? 5000;

	let view = $state<SystemView | null>(null);
	let loading = $state(true);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	async function refresh(): Promise<void> {
		try {
			view = await readSystem();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
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
		get system(): SystemView | null {
			return view;
		},
		/** Skeletons only before the first payload; a later failure keeps the snapshot. */
		get showSkeletons(): boolean {
			return loading && view === null;
		},
		/** Why the last read failed, as opposed to the server's own partial-failure note. */
		get lastError(): string {
			return error;
		},
		/**
		 * The server's own note that part of the snapshot failed. It arrives on a
		 * 200, so it is a warning about missing rows rather than a broken page.
		 */
		get partialError(): string {
			return view?.error ?? '';
		},
		/** Never `undefined`: the list is absent off Linux, which is not an empty host. */
		get processes(): SystemProcess[] {
			return view?.processes ?? [];
		},
		refresh,
		start,
		stop
	};
}

export type SystemStore = ReturnType<typeof createSystemStore>;
