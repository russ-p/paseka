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
});
