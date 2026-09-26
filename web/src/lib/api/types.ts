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

/** Mirrors `console.GitCommitView`, one commit the default branch has but origin does not. */
export interface GitCommit {
	sha: string;
	subject: string;
}

/** Mirrors `console.GitWorktreeView`, one colony-managed isolated checkout. */
export interface GitWorktree {
	traceId?: string;
	path: string;
	branch?: string;
	baseSha?: string;
	dirty: boolean;
	prUrl?: string;
}

/** Mirrors `console.GitBranchView`, one local branch. */
export interface GitBranch {
	name: string;
	current: boolean;
	default: boolean;
	merged: boolean;
	worktreePath?: string;
	traceId?: string;
	subject?: string;
	/** Merged, prefixed `paseka/`/`feature/`/`hotfix/`/`fix/`, not the default branch, no worktree. */
	leftover: boolean;
}

/**
 * Mirrors `console.GitView` from GET /api/git. The header fields are
 * `console.GitPlaqueView` flattened in, so one payload serves the topbar plaque
 * and this page.
 */
export interface GitView extends GitPlaque {
	unpublished?: GitCommit[];
	worktrees: GitWorktree[];
	branches: GitBranch[];
}

/** Mirrors `console.GitBranchDeleteItem`, the per-name outcome of a branch delete. */
export interface GitBranchDeleteItem {
	name: string;
	ok: boolean;
	error?: string;
}

/**
 * Mirrors `console.GitActionResult`, the answer to every git POST. `message` is
 * whatever git printed and is often empty; a batch delete reports per name in
 * `results` instead.
 */
export interface GitActionResult {
	ok: boolean;
	message?: string;
	results?: GitBranchDeleteItem[];
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

/** Mirrors `protocol.Usage`, the token accounting one adapter run reports. */
export interface Usage {
	inputTokens: number;
	outputTokens: number;
	cacheReadTokens?: number;
	cacheWriteTokens?: number;
	durationMs?: number;
	source?: string;
}

/** Mirrors `runs.UsageAggregate`, the token accounting summed over a trace. */
export interface UsageAggregate {
	inputTokens: number;
	outputTokens: number;
	cacheReadTokens: number;
	cacheWriteTokens: number;
	runCountWithUsage: number;
}

/**
 * The honey reserve a trail carries. A standing trail is capped by its stipend;
 * any other trail by whatever top-ups have been added. Mirrors the
 * `Energy*`/`LowEnergy`/`Standing` fields of `hiveview.TraceSummaryView`.
 */
export interface HoneyReserve {
	energyBudget?: number;
	energyRemaining?: number;
	energyAdded?: number;
	energyAllocated?: number;
	lowEnergy?: boolean;
	standing?: boolean;
}

/** Mirrors `hiveview.TraceSummaryView`. */
export interface TraceSummary extends HoneyReserve {
	traceId: string;
	title?: string;
	summary?: string;
	lastActivityAt: string;
	runCount: number;
	taskCount: number;
	bees?: string[];
	hasFailures: boolean;
	hasActive: boolean;
	usage?: UsageAggregate;
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
	usage?: Usage;
	runDir: string;
	startedAt: string;
	finishedAt?: string;
	hasEvents: boolean;
	hasSession: boolean;
}

/** Mirrors `hiveview.TaskSummaryView`. */
export interface TaskSummary {
	taskId: string;
	title: string;
	status: string;
	bee?: string;
}

/** Mirrors `hiveview.WorktreeView`. */
export interface Worktree {
	traceId: string;
	path: string;
	baseSha: string;
	branch?: string;
	createdAt: string;
}

/** Mirrors `hiveview.PullRequestView`. */
export interface PullRequest {
	url?: string;
	number?: number;
	head?: string;
	state?: string;
	draft?: boolean;
}

/** Fields `SignalCard` needs from any SIGNAL/INSIGHT/MUTATION/VERIFICATION feed row. */
export interface SignalSummary {
	createdAt: string;
	traceId: string;
	agentId: string;
	bee?: string;
	payloadKind?: string;
	/** Event type, set on feed rows; absent on the dashboard's insight projection. */
	type?: string;
	summary: string;
	severity?: string;
}

/** Mirrors `hiveview.EventFeedItem`. */
export interface EventFeedItem extends SignalSummary {
	id: string;
	type: string;
	taskId?: string;
}

/**
 * Mirrors `hiveview.TraceDetailView`; the embedded `TraceSummaryView` fields
 * are flattened into the same object. `tasks`, `runs`, and `recentEvents` are
 * `null` for an empty projection.
 */
export interface TraceDetail extends TraceSummary {
	tasks: TaskSummary[] | null;
	runs: RunSummary[] | null;
	worktree?: Worktree;
	pullRequest?: PullRequest;
	recentEvents: EventFeedItem[] | null;
}

/** Mirrors `hiveview.ArtifactView`, one trail comb file. */
export interface ArtifactView {
	ref: string;
	artifactKind: string;
	title?: string;
	updated?: number;
	producer?: string;
	announced: boolean;
	staged: boolean;
}

/**
 * Mirrors `hiveview.ArtifactContentView`. The server refuses to inline binary
 * or oversized bodies, so `omitted` replaces both content fields instead of
 * truncating them.
 */
export interface ArtifactContent {
	ref: string;
	content?: string;
	contentHtml?: string;
	omitted?: string;
}

/** Mirrors `hiveview.InsightHighlight`. */
export interface InsightHighlight extends SignalSummary {
	payloadKind: string;
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

/** Mirrors `console.EnergyAddResponse` from POST /api/traces/:id/energy/add. */
export interface EnergyAddResult {
	traceId: string;
	amount: number;
	energyBudget: number;
	energyRemaining: number;
	energyAdded: number;
	energyAllocated: number;
	lowEnergy: boolean;
}
