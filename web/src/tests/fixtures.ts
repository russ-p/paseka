import type {
	DashboardSummary,
	InsightHighlight,
	RunSummary,
	TraceSummary
} from '$lib/api/types';

export function traceSummary(overrides: Partial<TraceSummary> = {}): TraceSummary {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		title: 'Refactor the adapter seam',
		summary: 'Move adapter construction behind one interface.',
		lastActivityAt: '2026-09-25T18:04:22Z',
		runCount: 4,
		taskCount: 2,
		bees: ['builder'],
		hasFailures: false,
		hasActive: true,
		...overrides
	};
}

export function runSummary(overrides: Partial<RunSummary> = {}): RunSummary {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'run-01',
		bee: 'builder',
		adapter: 'opencode',
		workspace: '.paseka/worktrees/trace-01a0bd6963faa14f',
		taskId: 'task-01',
		state: 'failed',
		runDir: '.paseka/runs/trace-01a0bd6963faa14f/run-01',
		startedAt: '2026-09-25T18:00:00Z',
		finishedAt: '2026-09-25T18:02:00Z',
		hasEvents: true,
		hasSession: false,
		...overrides
	};
}

export function insightHighlight(overrides: Partial<InsightHighlight> = {}): InsightHighlight {
	return {
		createdAt: '2026-09-25T18:03:00Z',
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'run-01',
		bee: 'builder',
		payloadKind: 'INSIGHT',
		summary: 'The seam is now the only place that knows about providers.',
		...overrides
	};
}

export function dashboardSummary(overrides: Partial<DashboardSummary> = {}): DashboardSummary {
	return {
		runtime: { status: 'running', alive: true, slug: 'paseka' },
		nats: { configured: true, connected: true, ok: true, url: 'nats://127.0.0.1:4222' },
		activeSessions: 1,
		activeWorktrees: 2,
		taskCounts: { done: 3, running: 1 },
		recentTraces: [traceSummary()],
		failedRuns: [runSummary()],
		recentInsights: [insightHighlight()],
		...overrides
	};
}
