import type {
	ArtifactView,
	DashboardSummary,
	EventFeedItem,
	InsightHighlight,
	RunSummary,
	TraceDetail,
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

export function artifactView(overrides: Partial<ArtifactView> = {}): ArtifactView {
	return {
		ref: '.paseka/runs/trace-01a0bd6963faa14f/artifacts/notes.md',
		artifactKind: 'note',
		title: 'Adapter seam notes',
		producer: 'run-01',
		announced: true,
		staged: false,
		...overrides
	};
}

export function traceDetail(overrides: Partial<TraceDetail> = {}): TraceDetail {
	return {
		...traceSummary({
			energyBudget: 12,
			energyRemaining: 8,
			energyAllocated: 12
		}),
		tasks: [
			{ taskId: 'task-a1', title: 'Introduce the Adapter interface', status: 'completed', bee: 'builder' },
			{ taskId: 'task-b2', title: 'Guard the seam', status: 'waiting_review', bee: 'guard' }
		],
		runs: [
			runSummary({ agentId: 'run-04', bee: 'builder', state: 'running' }),
			runSummary({
				agentId: 'run-03',
				bee: 'guard',
				state: 'failed',
				finishedAt: '2026-09-25T18:02:00Z',
				usage: { inputTokens: 1200, outputTokens: 340, cacheReadTokens: 800 }
			})
		],
		worktree: {
			traceId: 'trace-01a0bd6963faa14f',
			path: '.paseka/worktrees/trace-01a0bd6963faa14f',
			baseSha: '03cd2afb188522ea31ae662dc9d7300883a7f531',
			branch: 'paseka/trace-01a0bd6963faa14f',
			createdAt: '2026-09-25T17:38:00Z'
		},
		recentEvents: Array.from({ length: 12 }, (_, index) =>
			eventFeedItem({
				id: `event-${index}`,
				summary: `Event number ${index}`,
				createdAt: '2026-09-25T18:04:22Z'
			})
		),
		...overrides
	};
}

export function eventFeedItem(overrides: Partial<EventFeedItem> = {}): EventFeedItem {
	return {
		id: '2026-09-25T18:04:22Z|trace-01a0bd6963faa14f|run-01|7',
		createdAt: '2026-09-25T18:04:22Z',
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'run-01',
		bee: 'builder',
		type: 'INSIGHT',
		payloadKind: 'seam.note',
		taskId: 'task-b2',
		severity: 'warning',
		summary: 'The cursor adapter still reaches for the provider registry directly.',
		...overrides
	};
}
