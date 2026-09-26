export interface RuntimeStatus {
	status: string;
	alive: boolean;
	slug?: string;
	colonyRoot?: string;
	pid?: number;
	startedAt?: string;
	lastHeartbeatAt?: string;
	subjectPrefix?: string;
}

export interface AgentItem {
	kind: string;
	bee: string;
	pid: number;
	traceId: string;
	agentId: string;
	sessionId?: string;
	startedAt: string;
	runDir: string;
}

export interface AgentsStatus {
	count: number;
	afk: number;
	sessions: number;
	items?: AgentItem[];
}

export interface AttentionStatus {
	reviews: number;
	sessions: number;
}

export interface HostStatus {
	hostname?: string;
	kernel?: string;
	os: string;
	arch: string;
	cpus: number;
	uptimeSeconds?: number;
	consolePid?: number;
	goVersion?: string;
	load1?: number;
	load5?: number;
	load15?: number;
	cpuPercent?: number;
	memUsedBytes?: number;
	memTotalBytes?: number;
	memAvailableBytes?: number;
	diskUsedBytes?: number;
	diskTotalBytes?: number;
	error?: string;
}

export interface GitPlaque {
	branch: string;
	headSha: string;
	headShaShort: string;
	dirty: boolean;
	defaultBranch: string;
	originUrl?: string;
	ahead?: number;
	behind?: number;
	lastFetchAgeSeconds?: number;
	note?: string;
}

export interface ChromeFrame {
	schemaVersion: number;
	runtime?: RuntimeStatus;
	runtimeError?: string;
	agents?: AgentsStatus;
	agentsError?: string;
	host?: HostStatus;
	hostError?: string;
	git?: GitPlaque;
	gitError?: string;
	attention?: AttentionStatus;
	attentionError?: string;
}

/** Mirrors `hiveview.TraceSummaryView`. */
export interface TraceSummary {
	traceId: string;
	title?: string;
	summary?: string;
	lastActivityAt: string;
	runCount: number;
	taskCount: number;
	bees?: string[];
	hasFailures: boolean;
	hasActive: boolean;
	energyBudget?: number;
	energyRemaining?: number;
	energyAdded?: number;
	energyAllocated?: number;
	lowEnergy?: boolean;
	standing?: boolean;
}

/** Mirrors `hiveview.RunView`. */
export interface RunSummary {
	traceId: string;
	agentId: string;
	bee: string;
	adapter: string;
	workspace: string;
	taskId?: string;
	state: string;
	summary?: string;
	runDir: string;
	startedAt: string;
	finishedAt?: string;
	hasEvents: boolean;
	hasSession: boolean;
}

/** Mirrors `hiveview.InsightHighlight`. */
export interface InsightHighlight {
	createdAt: string;
	traceId: string;
	agentId: string;
	bee?: string;
	payloadKind: string;
	summary: string;
	severity?: string;
}

export interface NATSStatusView {
	configured: boolean;
	connected: boolean;
	ok: boolean;
	url?: string;
	errors?: string[];
}

/** Mirrors `console.DashboardView` from GET /api/dashboard. `runtime` is `hiveview.RuntimeView`, the same shape as the chrome stream. */
export interface DashboardSummary {
	runtime?: RuntimeStatus;
	nats?: NATSStatusView;
	activeSessions: number;
	activeWorktrees: number;
	taskCounts: Record<string, number>;
	recentTraces: TraceSummary[];
	failedRuns: RunSummary[];
	recentInsights: InsightHighlight[];
}

/** Mirrors `console.CueView` from GET /api/cues. */
export interface Cue {
	id: string;
	description?: string;
	standingTrace?: string;
}

/** Mirrors `console.RunCueResponse` from POST /api/cues/:id/run. */
export interface RunCueResult {
	traceId: string;
	taskId?: string;
	eventType?: string;
	kind?: string;
}
