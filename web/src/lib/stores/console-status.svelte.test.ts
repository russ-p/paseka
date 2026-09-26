import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ChromeFrame } from '$lib/api/types';
import { createConsoleStatusStore } from './console-status.svelte';

class FakeEventSource {
	static latest: FakeEventSource | null = null;

	onopen: ((event: Event) => void) | null = null;
	onerror: ((event: Event) => void) | null = null;
	closed = false;
	private chrome: ((event: MessageEvent<string>) => void) | null = null;

	constructor(readonly url: string) {
		FakeEventSource.latest = this;
	}

	addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void {
		if (type === 'chrome') this.chrome = listener;
	}

	close(): void {
		this.closed = true;
	}

	open(): void {
		this.onopen?.(new Event('open'));
	}

	emit(frame: ChromeFrame): void {
		this.chrome?.(new MessageEvent('chrome', { data: JSON.stringify(frame) }));
	}

	fail(): void {
		this.onerror?.(new Event('error'));
	}
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('consoleStatusStore', () => {
	it('merges chrome frames and dashboard summaries', async () => {
		vi.stubGlobal('EventSource', FakeEventSource);
		const loadDashboard = vi.fn(async () => ({
			nats: { configured: true, connected: true, ok: true },
			recentTraces: [
				{ traceId: 'trace-active', hasActive: true },
				{ traceId: 'trace-idle', hasActive: false }
			]
		}));
		const store = createConsoleStatusStore({ loadDashboard, pollIntervalMs: 0 });

		store.start();
		const source = FakeEventSource.latest;
		if (!source) throw new Error('EventSource was not created');
		expect(source.url).toBe('/api/chrome/stream');
		source.open();
		source.emit({
			schemaVersion: 1,
			runtime: { status: 'running', alive: true, slug: 'demo' },
			agents: { count: 2, afk: 1, sessions: 1 },
			attention: { reviews: 3, sessions: 1 }
		});

		await vi.waitFor(() => expect(store.natsStatus).toBe('connected'));
		expect(store.connection).toBe('connected');
		expect(store.runtimeStatus).toBe('running');
		expect(store.colony).toBe('demo');
		expect(store.liveBees).toBe(2);
		expect(store.reviews).toBe(3);
		expect(store.activeTraceCount).toBe(1);
		expect(store.loading).toBe(false);

		source.fail();
		expect(store.connection).toBe('reconnecting');
		store.stop();
		expect(source.closed).toBe(true);
	});

	it('surfaces frame errors and missing EventSource support', async () => {
		vi.stubGlobal('EventSource', undefined);
		const store = createConsoleStatusStore({
			loadDashboard: async () => {
				throw new Error('dashboard unavailable');
			},
			pollIntervalMs: 0
		});

		store.applyChromeFrame({ schemaVersion: 2 });
		expect(store.lastError).toContain('unsupported chrome schema');

		store.start();
		await vi.waitFor(() => expect(store.lastError).toContain('dashboard unavailable'));
		expect(store.connection).toBe('disconnected');
		store.stop();
	});

	it('keeps host and git projections from the chrome frame', () => {
		const store = createConsoleStatusStore({ pollIntervalMs: 0 });

		store.applyChromeFrame({
			schemaVersion: 1,
			host: { os: 'linux', arch: 'amd64', cpus: 16, hostname: 'thinkpad' },
			git: { branch: 'main', headSha: 'abc123', headShaShort: 'abc', dirty: true, defaultBranch: 'main' }
		});
		expect(store.host?.hostname).toBe('thinkpad');
		expect(store.git?.branch).toBe('main');
		expect(store.gitError).toBe('');

		store.applyChromeFrame({ schemaVersion: 1, hostError: 'collect failed', gitError: 'git failed' });
		expect(store.hostError).toBe('collect failed');
		expect(store.gitError).toBe('git failed');
	});

	it('starts and stops the runtime through the injected calls', async () => {
		const start = vi.fn(async () => ({ status: 'running', alive: true, pid: 7, slug: 'demo' }));
		const stop = vi.fn(async () => ({ status: 'stopped', alive: false, slug: 'demo' }));
		const store = createConsoleStatusStore({ runtime: { start, stop }, pollIntervalMs: 0 });

		store.applyChromeFrame({ schemaVersion: 1, runtime: { status: 'stopped', alive: false } });

		expect(await store.startRuntime()).toBe(true);
		expect(start).toHaveBeenCalledTimes(1);
		expect(store.runtimeStatus).toBe('running');
		expect(store.runtime?.pid).toBe(7);
		expect(store.runtimeAction).toBeNull();

		expect(await store.stopRuntime()).toBe(true);
		expect(stop).toHaveBeenCalledTimes(1);
		expect(store.runtimeStatus).toBe('stopped');
		expect(store.runtimeError).toBe('');
	});

	it('records runtime failures and ignores overlapping actions', async () => {
		let release: (() => void) | undefined;
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		const stop = vi.fn(async () => {
			await gate;
			return { status: 'stopped', alive: false };
		});
		const store = createConsoleStatusStore({ pollIntervalMs: 0 });
		const failing = createConsoleStatusStore({
			runtime: {
				start: async () => {
					throw new Error('runtime start failed: 500');
				},
				stop
			},
			pollIntervalMs: 0
		});

		expect(await failing.startRuntime()).toBe(false);
		expect(failing.runtimeError).toBe('runtime start failed: 500');

		failing.clearRuntimeError();
		expect(failing.runtimeError).toBe('');

		const first = failing.stopRuntime();
		expect(failing.runtimeAction).toBe('stopping');
		expect(await failing.stopRuntime()).toBe(false);
		expect(stop).toHaveBeenCalledTimes(1);

		release?.();
		expect(await first).toBe(true);
		expect(failing.runtimeAction).toBeNull();
		expect(store.runtimeAction).toBeNull();
	});
});
