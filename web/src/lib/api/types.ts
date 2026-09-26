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

export interface DashboardSummary {
	nats?: {
		configured: boolean;
		connected: boolean;
		ok: boolean;
		url?: string;
		errors?: string[];
	};
	recentTraces?: Array<{
		traceId: string;
		hasActive?: boolean;
	}>;
}
