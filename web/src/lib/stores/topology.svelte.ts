import { getTopology as requestTopology } from '$lib/api/client';
import type { Topology } from '$lib/api/types';

interface TopologyStoreOptions {
	loadTopology?: () => Promise<Topology>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony topology behind `/next/topology`. It does not poll, like the event
 * feed: the projection is derived from committed bee YAML and colony
 * `auto_invites`, so it changes when a commit lands, not on a clock. Polling would
 * spend a filesystem walk and a YAML parse per operator for data that is static
 * between deploys, so Refresh is the honest control.
 */
export function createTopologyStore(options: TopologyStoreOptions = {}) {
	const readTopology = options.loadTopology ?? requestTopology;

	let topology = $state<Topology | null>(null);
	let loading = $state(true);
	let error = $state('');
	let started = false;

	async function refresh(): Promise<void> {
		loading = true;
		try {
			topology = await readTopology();
			error = '';
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	/** Guarded, so a remount or a re-run effect does not read the projection twice. */
	function start(): void {
		if (started) return;
		started = true;
		void refresh();
	}

	return {
		get topology(): Topology | null {
			return topology;
		},
		/** Skeletons only before the first payload; a later failure keeps the graph. */
		get showSkeletons(): boolean {
			return loading && topology === null;
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		/**
		 * A colony with no bees and no events has nothing to draw, which is different
		 * from a failure to read the projection.
		 */
		get isEmpty(): boolean {
			const view = topology;
			if (!view) return false;
			return view.bees.length === 0 && view.events.length === 0;
		},
		refresh,
		start
	};
}

export type TopologyStore = ReturnType<typeof createTopologyStore>;
