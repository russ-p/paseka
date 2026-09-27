import { getConfig as requestConfig } from '$lib/api/client';
import type { ColonyConfig } from '$lib/api/types';

interface SettingsStoreOptions {
	loadConfig?: () => Promise<ColonyConfig>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The colony's effective configuration behind `/next/settings`.
 *
 * **It polls nothing, and that is the finding rather than an omission.** A bee
 * roster is derived from committed YAML and the topology from `auto_invites`, so
 * both land on a commit — the reasoning that gave the Bees route a deliberate
 * Refresh instead of a timer. Configuration is the same shape one step further
 * out: it changes when an operator edits a file, and the one thing that does move
 * under a running process — whether NATS is connected — arrives on the chrome
 * stream the topbar already keeps, which this page reads rather than re-asking.
 *
 * So a timer here would re-read identical values forever, and a page that claimed
 * to be live would be lying about the one field that moves. Refresh is the honest
 * control, and it is the same word the Bees page uses.
 *
 * A failure keeps the last good snapshot: the values an operator is reading are
 * still the last real ones, and the alert says they stopped moving.
 */
export function createSettingsStore(options: SettingsStoreOptions = {}) {
	const readConfig = options.loadConfig ?? requestConfig;

	let config = $state<ColonyConfig | null>(null);
	let loading = $state(true);
	let error = $state('');
	let started = false;

	async function refresh(): Promise<void> {
		loading = true;
		try {
			config = await readConfig();
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
	}

	function stop(): void {
		started = false;
	}

	return {
		get config(): ColonyConfig | null {
			return config;
		},
		/** Skeletons only before the first payload; a later failure keeps the snapshot. */
		get showSkeletons(): boolean {
			return loading && config === null;
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return error;
		},
		/** Never `undefined`: an absent adapter list is not a colony with no adapters. */
		get adapters() {
			return config?.adapters ?? [];
		},
		refresh,
		start,
		stop
	};
}

export type SettingsStore = ReturnType<typeof createSettingsStore>;
