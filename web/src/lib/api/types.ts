export interface RuntimeStatus {
	status: string;
	alive: boolean;
	slug?: string;
	colonyRoot?: string;
}

export interface AgentsStatus {
	count: number;
	afk: number;
	sessions: number;
}

export interface AttentionStatus {
	reviews: number;
	sessions: number;
}

export interface ChromeFrame {
	schemaVersion: number;
	runtime?: RuntimeStatus;
	runtimeError?: string;
	agents?: AgentsStatus;
	agentsError?: string;
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
