import type {
	ArtifactView,
	DashboardSummary,
	EventFeedItem,
	EventFeedPage,
	GitBranch,
	GitCommit,
	GitView,
	GitWorktree,
	InsightHighlight,
	ProtocolEvent,
	RunSummary,
	SystemProcess,
	SystemView,
	Topology,
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
	const item: EventFeedItem = {
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
		raw: {
			protocolVersion: '1',
			traceId: 'trace-01a0bd6963faa14f',
			agentId: 'run-01',
			seq: 7,
			type: 'INSIGHT',
			createdAt: '2026-09-25T18:04:22Z',
			payload: { kind: 'seam.note', severity: 'warning' }
		},
		...overrides
	};
	return item;
}

export function eventFeedPage(
	items: EventFeedItem[] = [eventFeedItem()],
	overrides: Partial<EventFeedPage> = {}
): EventFeedPage {
	return { items, hasMore: false, ...overrides };
}

export function gitCommit(overrides: Partial<GitCommit> = {}): GitCommit {
	return {
		sha: '02453e88445ce591e01200c507bce35bc5071656',
		subject: 'feat(console): migrate the traces list and trail detail to /next',
		...overrides
	};
}

export function gitWorktree(overrides: Partial<GitWorktree> = {}): GitWorktree {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		path: '/colony/.paseka/worktrees/trace-01a0bd6963faa14f',
		branch: 'paseka/trace-01a0bd6963faa14f',
		baseSha: '03cd2afb188522ea31ae662dc9d7300883a7f531',
		dirty: false,
		...overrides
	};
}

export function gitBranch(overrides: Partial<GitBranch> = {}): GitBranch {
	return {
		name: 'main',
		current: true,
		default: true,
		merged: false,
		subject: 'Merge the console redesign',
		leftover: false,
		...overrides
	};
}

/** The full `GET /api/git` payload: compared against origin, three commits to publish. */
export function gitView(overrides: Partial<GitView> = {}): GitView {
	return {
		branch: 'main',
		headSha: '02453e88445ce591e01200c507bce35bc5071656',
		headShaShort: '02453e8',
		dirty: true,
		defaultBranch: 'main',
		originUrl: 'git@github.com:russ-p/paseka.git',
		ahead: 3,
		behind: 0,
		lastFetchAgeSeconds: 524_156,
		unpublished: [
			gitCommit(),
			gitCommit({
				sha: '70b7bedf3fb1f645f3082f3fe0f5d459e7bd9d11',
				subject: 'docs: record the dashboard grid contract'
			})
		],
		worktrees: [
			gitWorktree(),
			gitWorktree({
				traceId: 'trace-01a09966fdbe6771',
				path: '/colony/.paseka/worktrees/trace-01a09966fdbe6771',
				branch: 'paseka/trace-01a09966fdbe6771',
				dirty: true,
				prUrl: 'https://github.com/russ-p/paseka/pull/42'
			})
		],
		branches: [
			gitBranch(),
			gitBranch({
				name: 'paseka/trace-019f76d17ca323c8',
				current: false,
				default: false,
				merged: true,
				leftover: true,
				subject: 'Add a prune cleanup command'
			}),
			gitBranch({
				name: 'paseka/trace-01a09966fdbe6771',
				current: false,
				default: false,
				worktreePath: '/colony/.paseka/worktrees/trace-01a09966fdbe6771',
				traceId: 'trace-01a09966fdbe6771',
				subject: 'Wire the export format flag'
			})
		],
		...overrides
	};
}

export function systemProcess(overrides: Partial<SystemProcess> = {}): SystemProcess {
	return {
		pid: 4242,
		rssBytes: 214_958_080,
		cpuPercent: 12.4,
		comm: 'agent',
		cmd: 'cursor-agent --print --output-format stream-json',
		...overrides
	};
}

export function systemView(overrides: Partial<SystemView> = {}): SystemView {
	return {
		hostname: 'apiary',
		kernel: '6.11.0-21-generic',
		os: 'linux',
		arch: 'amd64',
		cpus: 8,
		uptimeSeconds: 604_800,
		consolePid: 1,
		goVersion: 'go1.25.1',
		load1: 1.42,
		load5: 0.98,
		load15: 0.61,
		cpuPercent: 18,
		memUsedBytes: 6_442_450_944,
		memTotalBytes: 17_179_869_184,
		memAvailableBytes: 10_737_418_240,
		diskUsedBytes: 42_949_672_960,
		diskTotalBytes: 214_748_364_800,
		processes: [
			systemProcess(),
			systemProcess({
				pid: 1187,
				rssBytes: 1_073_741_824,
				cpuPercent: 41.8,
				comm: 'java',
				cmd: 'java -jar node_modules/.bin/jest --runInBand'
			}),
			systemProcess({
				pid: 909,
				rssBytes: 3_145_728,
				comm: 'paseka',
				cmd: 'paseka console --addr :8787'
			})
		],
		...overrides
	};
}

export function topology(overrides: Partial<Topology> = {}): Topology {
	return {
		bees: [
			{ role: 'scout', adapter: 'cursor-agent', intents: ['discovery', 'triage'] },
			{ role: 'builder', adapter: 'claude-code', intents: ['implementation'] },
			{ role: 'drone', adapter: 'pi', defaultIntent: 'grilling' }
		],
		events: [
			{ id: 'SIGNAL/task.ready', type: 'SIGNAL', kind: 'task.ready' },
			{ id: 'SIGNAL/feature.classified', type: 'SIGNAL', kind: 'feature.classified' },
			{ id: 'INSIGHT/review.note', type: 'INSIGHT', kind: 'review.note' },
			{ id: 'MUTATION/code.proposal.isolated', type: 'MUTATION', kind: 'code.proposal.isolated' },
			{ id: 'VERIFICATION/verification.success', type: 'VERIFICATION', kind: 'verification.success' }
		],
		edges: [
			// No `subscribes` declared, so the projection synthesises this implicit rule.
			{ kind: 'subscribe', from: 'SIGNAL/task.ready', to: 'scout', dispatch: 'task', implicit: true },
			{
				kind: 'subscribe',
				from: 'INSIGHT/review.note',
				to: 'builder',
				dispatch: 'direct'
			},
			{
				kind: 'invite',
				from: 'SIGNAL/feature.classified',
				to: 'drone',
				intent: 'grilling',
				match: { decision: 'grill' }
			},
			{ kind: 'publish', from: 'builder', to: 'MUTATION/code.proposal.isolated' },
			{ kind: 'publish', from: 'guard', to: 'VERIFICATION/verification.success' }
		],
		mermaid: 'flowchart LR\n  bee:scout --> event:task.ready\n',
		...overrides
	};
}

/** A raw `protocol.Event` as the run events endpoint returns it. */
export function protocolEvent(overrides: Partial<ProtocolEvent> = {}): ProtocolEvent {
	return {
		protocolVersion: '1',
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'run-01',
		seq: 1,
		type: 'INSIGHT',
		createdAt: '2026-09-25T18:00:30Z',
		payload: { kind: 'run.summary', summary: 'Committed the prune cleanup command.' },
		...overrides
	};
}

/** Three runs of one trail, newest first, plus a run from another trail. */
export function runList(): RunSummary[] {
	return [
		runSummary({
			agentId: 'run-03',
			state: 'running',
			startedAt: '2026-09-25T18:06:00Z',
			finishedAt: undefined,
			summary: undefined,
			hasEvents: false
		}),
		runSummary({
			agentId: 'run-02',
			state: 'completed',
			startedAt: '2026-09-25T18:03:00Z',
			finishedAt: '2026-09-25T18:05:00Z',
			summary: 'Wired the export format flag.'
		}),
		runSummary({
			agentId: 'run-01',
			adapter: 'claude-code',
			intent: 'debugging',
			state: 'failed',
			startedAt: '2026-09-25T18:00:00Z',
			finishedAt: '2026-09-25T18:02:00Z',
			body: 'Fix the flaky retry test.',
			usage: { inputTokens: 12_000, outputTokens: 900, cacheReadTokens: 40_000 }
		}),
		runSummary({
			traceId: 'trace-01a09966fdbe6771',
			agentId: 'run-09',
			state: 'completed',
			startedAt: '2026-09-25T17:00:00Z',
			finishedAt: '2026-09-25T17:01:00Z'
		})
	];
}
