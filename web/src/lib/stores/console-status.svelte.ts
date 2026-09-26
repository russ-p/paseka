import type {
	AgentsStatus,
	AttentionStatus,
	ChromeFrame,
	DashboardSummary,
	GitPlaque,
	HostStatus,
	RuntimeStatus
} from '$lib/api/types';

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected';
export type RuntimeAction = 'starting' | 'stopping' | null;

interface RuntimeCalls {
	start: () => Promise<RuntimeStatus>;
	stop: () => Promise<RuntimeStatus>;
}

interface ConsoleStatusOptions {
	loadDashboard?: () => Promise<DashboardSummary>;
	runtime?: Partial<RuntimeCalls>;
	pollIntervalMs?: number;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

async function postRuntime(action: 'start' | 'stop'): Promise<RuntimeStatus> {
	const response = await fetch(`/api/runtime/${action}`, { method: 'POST' });
	if (!response.ok) throw new Error(`runtime ${action} failed: ${response.status}`);
	return (await response.json()) as RuntimeStatus;
}

async function loadDashboard(): Promise<DashboardSummary> {
	const response = await fetch('/api/dashboard');
	if (!response.ok) throw new Error(`dashboard request failed: ${response.status}`);
	return (await response.json()) as DashboardSummary;
}

export function createConsoleStatusStore(options: ConsoleStatusOptions = {}) {
	const readDashboard = options.loadDashboard ?? loadDashboard;
	const startRuntimeCall = options.runtime?.start ?? (() => postRuntime('start'));
	const stopRuntimeCall = options.runtime?.stop ?? (() => postRuntime('stop'));
	const pollIntervalMs = options.pollIntervalMs ?? 15000;

	let connection = $state<ConnectionStatus>('reconnecting');
	let runtime = $state<RuntimeStatus | null>(null);
	let agents = $state<AgentsStatus | null>(null);
	let attention = $state<AttentionStatus | null>(null);
	let host = $state<HostStatus | null>(null);
	let git = $state<GitPlaque | null>(null);
	let nats = $state<DashboardSummary['nats'] | undefined>(undefined);
	let activeTraceCount = $state(0);
	let loading = $state(true);
	let frameError = $state('');
	let hostError = $state('');
	let gitError = $state('');
	let dashboardError = $state('');
	let runtimeError = $state('');
	let runtimeAction = $state<RuntimeAction>(null);
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
		if (frame.host) host = frame.host;
		if (frame.git) git = frame.git;
		hostError = frame.hostError ?? '';
		gitError = frame.gitError ?? '';
		frameError = frame.runtimeError || frame.agentsError || frame.attentionError || '';
		connection = 'connected';
		loading = false;
	}

	function applyDashboard(summary: DashboardSummary): void {
		nats = summary.nats;
		activeTraceCount = summary.recentTraces?.filter((trace) => trace.hasActive).length ?? 0;
		loading = false;
	}

	async function runRuntimeAction(action: Exclude<RuntimeAction, null>, call: () => Promise<RuntimeStatus>): Promise<boolean> {
		if (runtimeAction !== null) return false;
		runtimeAction = action;
		runtimeError = '';
		try {
			runtime = await call();
			return true;
		} catch (error) {
			runtimeError = errorMessage(error);
			return false;
		} finally {
			runtimeAction = null;
		}
	}

	function startRuntime(): Promise<boolean> {
		return runRuntimeAction('starting', startRuntimeCall);
	}

	function stopRuntime(): Promise<boolean> {
		return runRuntimeAction('stopping', stopRuntimeCall);
	}

	function clearRuntimeError(): void {
		runtimeError = '';
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
		get runtime(): RuntimeStatus | null {
			return runtime;
		},
		get runtimeStatus(): string {
			return runtime?.status ?? 'unknown';
		},
		get runtimeAction(): RuntimeAction {
			return runtimeAction;
		},
		get runtimeError(): string {
			return runtimeError;
		},
		get colony(): string | undefined {
			return runtime?.slug;
		},
		get agents(): AgentsStatus | null {
			return agents;
		},
		get liveBees(): number {
			return agents?.count ?? 0;
		},
		get host(): HostStatus | null {
			return host;
		},
		get hostError(): string {
			return hostError;
		},
		get git(): GitPlaque | null {
			return git;
		},
		get gitError(): string {
			return gitError;
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
		clearRuntimeError,
		startRuntime,
		stopRuntime,
		start,
		stop
	};
}

export type ConsoleStatusStore = ReturnType<typeof createConsoleStatusStore>;

export const consoleStatusStore = createConsoleStatusStore();
