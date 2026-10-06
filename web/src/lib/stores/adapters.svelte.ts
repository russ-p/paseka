import { getAdapterCLIs as requestAdapterCLIs } from '$lib/api/client';
import type { AdapterCLI, AdapterCLIs } from '$lib/api/types';

interface AdapterCLIsStoreOptions {
	loadAdapters?: (refresh: boolean) => Promise<AdapterCLIs>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The Agent CLIs block's probe behind `/next/system`.
 *
 * It is the console's one store that **does not read on mount**. The block is
 * folded, the probe execs four external binaries, and the server caches the
 * answer until somebody asks for a new one — so arriving on System costs no
 * request at all, and opening the accordion costs one. There is no poll either:
 * an installed CLI changes when a human installs it, which is what `refresh` is
 * for.
 *
 * A read that fails keeps `loaded` false, so folding and reopening the block is
 * a retry rather than a stuck error over an empty table.
 */
export function createAdapterCLIsStore(options: AdapterCLIsStoreOptions = {}) {
	const readAdapters = options.loadAdapters ?? requestAdapterCLIs;

	let view = $state<AdapterCLIs | null>(null);
	let loading = $state(false);
	let error = $state('');
	let loaded = false;

	async function load(refresh = false): Promise<void> {
		// One read at a time: the accordion and its Refresh are the same block, and
		// a second probe run would exec the same four binaries twice.
		if (loading) return;
		if (loaded && !refresh) return;
		loading = true;
		error = '';
		try {
			view = await readAdapters(refresh);
			loaded = true;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	return {
		/** Never `undefined`: an absent list is not an empty machine. */
		get adapters(): AdapterCLI[] {
			return view?.adapters ?? [];
		},
		/** Whether a probe has answered yet — the summary note says not-probed until it has. */
		get probed(): boolean {
			return view !== null;
		},
		get loading(): boolean {
			return loading;
		},
		/** Why the last read failed; the rows it would have replaced stay on screen. */
		get lastError(): string {
			return error;
		},
		load,
		refresh: () => load(true)
	};
}

export type AdapterCLIsStore = ReturnType<typeof createAdapterCLIsStore>;
