import { getVersion as requestVersion } from '$lib/api/client';
import type { BuildView } from '$lib/api/types';

interface VersionStoreOptions {
	loadVersion?: () => Promise<BuildView>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * Which Paseka this console is talking to.
 *
 * It is a build fact, not a colony one, so there is no timer here: the answer
 * cannot change while the page is open, and a poller would keep re-reading a
 * constant. The side menu's stamp reads this one store, so the whole app names
 * the same build from one request.
 *
 * A failed read leaves the stamp absent rather than blanking it, because there is
 * nothing to show until it succeeds — and the footer says so quietly instead of
 * claiming a version the console does not have.
 */
export function createVersionStore(options: VersionStoreOptions = {}) {
	const loadVersion = options.loadVersion ?? requestVersion;

	let build = $state<BuildView | null>(null);
	let error = $state('');
	let started = false;

	async function refresh(): Promise<void> {
		try {
			build = await loadVersion();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		}
	}

	function start(): void {
		if (started) return;
		started = true;
		void refresh();
	}

	return {
		get build(): BuildView | null {
			return build;
		},
		/** Why the read failed. Silence is the default: an absent stamp says nothing. */
		get lastError(): string {
			return error;
		},
		refresh,
		start
	};
}

export type VersionStore = ReturnType<typeof createVersionStore>;

export const versionStore = createVersionStore();
