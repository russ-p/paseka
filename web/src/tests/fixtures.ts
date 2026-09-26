import type {
	ArtifactView,
	Bee,
	DashboardSummary,
	EventFeedItem,
	EventFeedPage,
	GitBranch,
	GitCommit,
	GitView,
	GitWorktree,
	InsightHighlight,
	ProtocolEvent,
	MergeDiff,
	ReviewQueue,
	ReviewQueueItem,
	RunSummary,
	SystemProcess,
	SystemView,
	TaskBoard,
	TaskDetail,
	TaskListItem,
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

/** A board row. The `can*` flags are the server's eligibility answer. */
export function taskListItem(overrides: Partial<TaskListItem> = {}): TaskListItem {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		taskId: 'task-01',
		title: 'Wire the export format flag',
		status: 'ready',
		review: 'none',
		bee: 'builder',
		sector: 'api',
		runCount: 0,
		canStart: true,
		canRetry: false,
		canApprove: false,
		canReject: false,
		isFinal: false,
		proposalWorkspace: 'isolated',
		updatedAt: '2026-09-25T18:00:00Z',
		...overrides
	};
}

/**
 * A board in server order: `ready` and `waiting_review` first, because the server
 * orders groups by the pipeline rather than alphabetically and a test that
 * reshuffled them would be testing the wrong thing.
 */
export function taskBoard(overrides: Partial<TaskBoard> = {}): TaskBoard {
	return {
		groups: [
			{
				status: 'ready',
				tasks: [taskListItem()]
			},
			{
				status: 'waiting_review',
				tasks: [
					taskListItem({
						taskId: 'task-02',
						title: 'Add the retry backoff',
						status: 'waiting_review',
						review: 'final',
						canStart: false,
						canApprove: true,
						canReject: true,
						runCount: 2,
						updatedAt: '2026-09-25T18:04:00Z'
					})
				]
			},
			{
				status: 'failed',
				tasks: [
					taskListItem({
						taskId: 'task-03',
						title: 'Fix the flaky retry test',
						status: 'failed',
						canStart: false,
						canRetry: true,
						dependsOn: ['task-01'],
						runCount: 3,
						updatedAt: '2026-09-25T17:30:00Z'
					})
				]
			}
		],
		taskCounts: { ready: 1, waiting_review: 1, failed: 1 },
		...overrides
	};
}

/** A detail: the board row plus body, summary, and the runs the task produced. */
export function taskDetail(overrides: Partial<TaskDetail> = {}): TaskDetail {
	return {
		...taskListItem(),
		body: 'Add an --format flag to `paseka export`.',
		intent: 'implementation',
		source: 'jetstream-kv',
		runs: [
			{
				agentId: 'run-02',
				bee: 'builder',
				runStatus: 'completed',
				runDir: '.paseka/runs/trace-01a0bd6963faa14f/run-02',
				startedAt: '2026-09-25T18:01:00Z',
				finishedAt: '2026-09-25T18:03:00Z'
			}
		],
		...overrides
	};
}

/** The interactive bees, which is what `GET /api/bees` returns. */
export function beeList(overrides: Partial<Bee> = {}): Bee[] {
	return [
		{
			role: 'builder',
			adapter: 'cursor',
			promptTemplate: 'implementation',
			worktree: true,
			intents: ['implementation', 'debugging']
		},
		{
			role: 'drone',
			adapter: 'pi',
			promptTemplate: 'grilling',
			worktree: false,
			intents: ['grilling'],
			defaultIntent: 'grilling',
			...overrides
		}
	];
}

/** One row of the review queue. */
export function reviewQueueItem(overrides: Partial<ReviewQueueItem> = {}): ReviewQueueItem {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		taskId: '002-merge-body-compose',
		title: 'Merge body compose + approve preview',
		review: 'required',
		summary: 'Composed the merge body from the trail summary.',
		bee: 'builder',
		sector: 'api',
		runCount: 1,
		updatedAt: '2026-09-25T18:00:00Z',
		isFinal: false,
		canApprove: true,
		canReject: true,
		...overrides
	};
}

/** A queue with a mid-trail review and a final gate, which are the two shapes. */
export function reviewQueue(overrides: Partial<ReviewQueue> = {}): ReviewQueue {
	return {
		items: [
			reviewQueueItem(),
			reviewQueueItem({
				taskId: '_review',
				title: 'Human review and merge',
				review: 'final',
				summary: 'Ready for a human to merge the trail.',
				isFinal: true,
				delivery: 'local_merge',
				proposalWorkspace: 'isolated',
				prTitle: 'Add paseka export --format',
				updatedAt: '2026-09-25T18:04:00Z'
			})
		],
		count: 2,
		...overrides
	};
}

/** A worktree diff as `GetMergeDiff` returns it. */
export function mergeDiff(overrides: Partial<MergeDiff> = {}): MergeDiff {
	return {
		traceId: 'trace-01a0bd6963faa14f',
		defaultBranch: 'main',
		branch: 'paseka/trace-01a0bd6963faa14f',
		baseSha: 'a'.repeat(40),
		headSha: 'b'.repeat(40),
		stat: ' web/src/lib/diff.ts | 3 +++\n 1 file changed, 3 insertions(+)\n',
		diff: [
			'diff --git a/web/src/lib/diff.ts b/web/src/lib/diff.ts',
			'index 1111111..2222222 100644',
			'--- a/web/src/lib/diff.ts',
			'+++ b/web/src/lib/diff.ts',
			'@@ -1,2 +1,3 @@',
			' export const first = 1;',
			' export const second = 2;',
			'+export const third = 3;',
			''
		].join('\n'),
		delivery: 'local_merge',
		...overrides
	};
}
