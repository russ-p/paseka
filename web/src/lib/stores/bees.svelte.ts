import { listBees as requestBees } from '$lib/api/client';
import type { Bee } from '$lib/api/types';

interface BeesStoreOptions {
	loadBees?: () => Promise<Bee[]>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony roster behind `/next/bees`, read at `?scope=colony` rather than the
 * launch picker: a roster that hides a bee because its adapter cannot be
 * started as a session would not be a roster.
 *
 * It does not poll, like the topology and the event feed. A bee's identity,
 * adapter, sector, and intent vocabulary come from committed bee YAML, so they
 * change when a commit lands, not on a clock — and the one fact that does move,
 * whether a bee is live, arrives on the chrome stream the topbar already keeps
 * and is joined in the page. What the read does carry is a last run, and that is
 * the reason Refresh is honest rather than a gap: a run landing is the only thing
 * here that changes without a commit.
 */
export function createBeesStore(options: BeesStoreOptions = {}) {
	const readBees = options.loadBees ?? (() => requestBees('colony'));

	let bees = $state<Bee[] | null>(null);
	let loading = $state(true);
	let error = $state('');
	let started = false;

	async function refresh(): Promise<void> {
		loading = true;
		try {
			bees = await readBees();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/** Guarded, so a remount or a re-run effect does not read the roster twice. */
	function start(): void {
		if (started) return;
		started = true;
		void refresh();
	}

	return {
		get bees(): Bee[] | null {
			return bees;
		},
		/** Skeletons only before the first payload; a later failure keeps the roster. */
		get showSkeletons(): boolean {
			return loading && bees === null;
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		/**
		 * A colony with no bee YAML has isolated nothing and offered nothing, which
		 * is a different claim from a failure to read the roster.
		 */
		get isEmpty(): boolean {
			return bees !== null && bees.length === 0;
		},
		refresh,
		start
	};
}

export type BeesStore = ReturnType<typeof createBeesStore>;
