import type { AgentsStatus, AttentionStatus, ChromeFrame, DashboardSummary, RuntimeStatus } from '$lib/api/types';

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected';

interface ConsoleStatusOptions {
	loadDashboard?: () => Promise<DashboardSummary>;
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

async function loadDashboard(): Promise<DashboardSummary> {
	const response = await fetch('/api/dashboard');
	if (!response.ok) throw new Error(`dashboard request failed: ${response.status}`);
	return (await response.json()) as DashboardSummary;
}

export function createConsoleStatusStore(options: ConsoleStatusOptions = {}) {
	const readDashboard = options.loadDashboard ?? loadDashboard;
	const pollIntervalMs = options.pollIntervalMs ?? 15000;

	let connection = $state<ConnectionStatus>('reconnecting');
	let runtime = $state<RuntimeStatus | null>(null);
	let agents = $state<AgentsStatus | null>(null);
	let attention = $state<AttentionStatus | null>(null);
	let nats = $state<DashboardSummary['nats'] | undefined>(undefined);
	let activeTraceCount = $state(0);
	let loading = $state(true);
	let frameError = $state('');
	let dashboardError = $state('');
	let source: EventSource | undefined;
	let timer: ReturnType<typeof setInterval> | undefined;
	let started = false;

	async function refreshDashboard(): Promise<void> {
		try {
			applyDashboard(await readDashboard());
			dashboardError = '';
		} catch (error) {
			dashboardError = errorMessage(error);
			loading = false;
		}
	}

	function applyChromeFrame(frame: ChromeFrame): void {
		if (frame.schemaVersion !== 1) {
			frameError = `unsupported chrome schema: ${frame.schemaVersion}`;
			loading = false;
			return;
		}
		if (frame.runtime) runtime = frame.runtime;
		if (frame.agents) agents = frame.agents;
		if (frame.attention) attention = frame.attention;
		frameError = frame.runtimeError || frame.agentsError || frame.attentionError || '';
		connection = 'connected';
		loading = false;
	}

	function applyDashboard(summary: DashboardSummary): void {
		nats = summary.nats;
		activeTraceCount = summary.recentTraces?.filter((trace) => trace.hasActive).length ?? 0;
		loading = false;
	}

	function start(): void {
		if (started) return;
		started = true;
		connection = 'reconnecting';
		void refreshDashboard();

		if (typeof EventSource === 'undefined') {
			connection = 'disconnected';
			return;
		}

		try {
			source = new EventSource('/api/chrome/stream');
			source.onopen = () => {
				connection = 'connected';
			};
			source.onerror = () => {
				connection = 'reconnecting';
			};
			source.addEventListener('chrome', (event) => {
				try {
					applyChromeFrame(JSON.parse((event as MessageEvent<string>).data) as ChromeFrame);
				} catch (error) {
					frameError = errorMessage(error);
					loading = false;
				}
			});
		} catch (error) {
			frameError = errorMessage(error);
			connection = 'disconnected';
		}

		if (pollIntervalMs > 0) {
			timer = setInterval(() => void refreshDashboard(), pollIntervalMs);
		}
	}

	function stop(): void {
		started = false;
		source?.close();
		source = undefined;
		if (timer !== undefined) clearInterval(timer);
		timer = undefined;
	}

	return {
		get connection(): ConnectionStatus {
			return connection;
		},
		get runtimeStatus(): string {
			return runtime?.status ?? 'unknown';
		},
		get colony(): string | undefined {
			return runtime?.slug;
		},
		get liveBees(): number {
			return agents?.count ?? 0;
		},
		get reviews(): number {
			return attention?.reviews ?? 0;
		},
		get pendingSessions(): number {
			return attention?.sessions ?? 0;
		},
		get natsStatus(): string {
			if (!nats) return 'unknown';
			if (!nats.configured) return 'idle';
			return nats.connected ? 'connected' : 'disconnected';
		},
		get activeTraceCount(): number {
			return activeTraceCount;
		},
		get loading(): boolean {
			return loading;
		},
		get lastError(): string {
			return dashboardError || frameError;
		},
		applyChromeFrame,
		applyDashboard,
		start,
		stop
	};
}

export type ConsoleStatusStore = ReturnType<typeof createConsoleStatusStore>;

export const consoleStatusStore = createConsoleStatusStore();
