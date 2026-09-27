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

/** Mirrors `console.SystemProcess`, one row of the process table. */
export interface SystemProcess {
	pid: number;
	rssBytes: number;
	/** A delta between two samples, so it is absent on the first poll. */
	cpuPercent?: number;
	comm: string;
	cmd: string;
}

/**
 * Mirrors `console.SystemView`. It extends the plaque's `HostStatus` because the
 * server embeds the very same struct in the chrome frame: the System route's
 * snapshot is the Host plaque plus the process list, not a parallel shape.
 */
export interface SystemView extends HostStatus {
	/** Absent off Linux, and when `/proc` could not be read. */
	processes?: SystemProcess[];
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
	/** The literal task text the adapter was handed. */
	body?: string;
	/** Prompt vocabulary the run was launched under. */
	intent?: string;
	state: string;
	summary?: string;
	usage?: Usage;
	/** The upstream session this run continued, when the adapter supports one. */
	providerSessionId?: string;
	runDir: string;
	startedAt: string;
	finishedAt?: string;
	hasEvents: boolean;
	hasSession: boolean;
}

/** Mirrors `console.EventsPage`, one page of a run's recorded events. */
export interface RunEventsPage {
	entries: ProtocolEvent[];
	/** An index, not a trace cursor; `0` means the beginning. */
	nextCursor: number;
}

/** Mirrors `hiveview.TaskSummaryView`. */
export interface TaskSummary {
	taskId: string;
	title: string;
	status: string;
	bee?: string;
}

/** Mirrors `hiveview.TaskRunView`: one adapter run a task produced. */
export interface TaskRun {
	agentId: string;
	bee?: string;
	runDir?: string;
	runStatus?: string;
	startedAt?: string;
	finishedAt?: string;
}

/**
 * Mirrors `hiveview.TaskListItem`, one row on the board.
 *
 * The `can*` flags are the server's answer to "what may the operator do with
 * this row", derived from the ledger's eligibility rules. The page asks the
 * server rather than re-deciding them, because a task's dependencies can change
 * underneath a stale board.
 */
export interface TaskListItem {
	traceId: string;
	taskId: string;
	title: string;
	status: string;
	/** Normalized, so `none` rather than empty. */
	review?: string;
	bee?: string;
	sector?: string;
	dependsOn?: string[];
	runCount: number;
	canStart: boolean;
	canRetry: boolean;
	canApprove: boolean;
	canReject: boolean;
	canRequestChanges?: boolean;
	reworkTaskId?: string;
	reworkStatus?: string;
	isFinal: boolean;
	/** `isolated` or `root`: where the bee's work is expected to land. */
	proposalWorkspace?: string;
	updatedAt?: string;
	/**
	 * `delivery`, `prTitle`, `prBody`, and `pullRequest` are populated only for a
	 * final task's detail. The board builds rows through the same projection as the
	 * plain trail view, so these never appear on one.
	 */
	delivery?: string;
	prTitle?: string;
	prBody?: string;
	pullRequest?: PullRequest;
}

/** Mirrors `hiveview.TaskDetailView`: a board row plus what only the task knows. */
export interface TaskDetail extends TaskListItem {
	/** The literal task text the bee is handed. */
	body?: string;
	intent?: string;
	/** The bee's own completion summary. */
	summary?: string;
	/** The trail's prose, on a final gate only. */
	traceSummary?: string;
	commit?: string;
	/**
	 * `null`, not `[]`, for a task no run has touched: Go marshals a nil slice as
	 * `null` and this field carries no `omitempty`, so a task planned by a planner
	 * and never dispatched answers with a null. `TraceDetail` says the same about its
	 * own lists, and every page normalises with `?? []`.
	 */
	runs: TaskRun[] | null;
	/** Where the ledger answered from: `jetstream-kv`, or `filesystem`. */
	source: string;
}

/**
 * Mirrors `hiveview.TaskBoardView`. `groups` arrives in lifecycle order from the
 * server, so the board keeps that order rather than sorting statuses itself.
 */
export interface TaskBoard {
	groups: { status: string; tasks: TaskListItem[] }[];
	/** `null` for a colony with no tasks at all, for the same nil-map reason as above. */
	taskCounts: Record<string, number> | null;
}

/** Mirrors `console.CreateTaskRequest`, the body of `POST /api/tasks`. */
export interface CreateTaskRequest {
	traceId?: string;
	taskId?: string;
	title?: string;
	body?: string;
	bee: string;
	sector?: string;
	intent?: string;
	dependsOn?: string[];
	review?: string;
	/** Publish `task.ready` on create, so the dispatcher picks it up at once. */
	autorun?: boolean;
}

/** Mirrors `console.CreateTaskResponse`. */
export interface CreateTaskResult {
	traceId: string;
	taskId: string;
	bee: string;
	autorun: boolean;
	message?: string;
}

/** Mirrors `console.StartTaskResponse`; `taskIds` is empty when one task was named. */
export interface StartTaskResult {
	traceId: string;
	taskIds: string[];
	message?: string;
}

/** Mirrors `console.RetryTaskResponse`. */
export interface RetryTaskResult {
	traceId: string;
	taskId: string;
	message?: string;
}

/**
 * Mirrors `console.SessionView`, one interactive session.
 *
 * `active` is a separate field rather than a reading of `state`, because the list is a
 * merge of three sources — the colony registry, this process's own sessions, and a scan
 * of `.paseka/runs` — and only the first two can be running. A session launched by
 * `paseka bee chat` in another shell is `state: "active"` with a live PID and no PTY
 * hub here, which is why `active` is what the console branches on.
 */
export interface Session {
	sessionId: string;
	traceId: string;
	agentId: string;
	bee: string;
	/** Decides whether the session can be resumed; the legacy read it and never showed it. */
	adapter?: string;
	workspace: string;
	/** `.paseka/runs/<traceId>/<agentId>` — where the transcript and artifacts land. */
	runDir: string;
	state: string;
	/** Present only while running. `Process.Kill` locally, `SIGTERM` across processes. */
	pid?: number;
	/** The upstream conversation a resume continues; without it there is nothing to resume. */
	providerSessionId?: string;
	/** The session this one continued, on a resume. */
	resumedFrom?: string;
	profile?: string;
	startedAt: string;
	finishedAt?: string;
	active: boolean;
}

/** Mirrors `runs.TranscriptEntry`, one line of `transcript.ndjson`. */
export interface TranscriptEntry {
	at: string;
	/** `user`, `agent`, or `system`. The session path only ever writes `agent` and `system`. */
	role: string;
	content: string;
}

/**
 * Mirrors `console.TranscriptPage`. `nextCursor` is an *entry index*, not a byte offset
 * or an opaque token, so it can be sent straight back as `after`.
 */
export interface TranscriptPage {
	entries: TranscriptEntry[];
	nextCursor: number;
}

/** Mirrors `hiveview.InviteView`, a task waiting for a human to start it. */
export interface Invite {
	inviteId: string;
	traceId: string;
	bee: string;
	intent?: string;
	task: string;
	status: string;
	artifactRef?: string;
	/** Set once an invite has been accepted into a session. */
	sessionId?: string;
	createdAt: string;
	updatedAt: string;
}

/** Mirrors `console.createSessionRequest`. */
export interface CreateSessionRequest {
	bee: string;
	/** The task, templated by the intent. Ignored when `useRawPrompt` is set. */
	body?: string;
	rawPrompt?: string;
	/** Omitted rather than sent blank: the server generates one. */
	traceId?: string;
	intent?: string;
	useRawPrompt?: boolean;
}

/**
 * What a stop actually did, which the two endpoints are not: a session in this process
 * is `Process.Kill`ed, a session in another is `SIGTERM`ed from the colony registry.
 * The legacy discarded this and the operator could not tell a hard kill from a request
 * to exit.
 */
export interface StopSessionResult {
	status: 'stopped' | 'signalled';
}

/** Mirrors the accept response: the invite, and the session it started when it started one. */
export interface AcceptInviteResult {
	inviteId: string;
	traceId: string;
	/** Absent when the invite is accepted but dispatched rather than launched. */
	sessionId?: string;
	session?: Session;
}

/** Mirrors `console.ReviewQueueItem`, one task waiting on a human. */
export interface ReviewQueueItem {
	traceId: string;
	taskId: string;
	title: string;
	review: string;
	summary?: string;
	/** The trail's prose, on a final gate only; also the merge commit body. */
	traceSummary?: string;
	bee?: string;
	sector?: string;
	runCount: number;
	updatedAt?: string;
	isFinal: boolean;
	proposalWorkspace?: string;
	/** `local_merge` or `pull_request`; decides whether approving merges or publishes. */
	delivery?: string;
	prTitle?: string;
	prBody?: string;
	pullRequest?: PullRequest;
	canApprove: boolean;
	canReject: boolean;
	/** False while a rework task from an earlier rejection is still in flight. */
	canRequestChanges?: boolean;
	reworkTaskId?: string;
	reworkStatus?: string;
}

/** Mirrors `console.ReviewQueueView`. */
export interface ReviewQueue {
	items: ReviewQueueItem[] | null;
	count: number;
}

/** Mirrors `console.MergeDiffView`: the worktree diff for a trace's final gate. */
export interface MergeDiff {
	traceId: string;
	defaultBranch: string;
	branch: string;
	baseSha: string;
	/** Pins a review to the commit it was read against; a moved head voids it. */
	headSha: string;
	/** Raw `git diff --stat`, parsed for the per-file counts. */
	stat?: string;
	/** Raw `git diff`, cut at the server's byte cap when `truncated` is set. */
	diff?: string;
	truncated?: boolean;
	empty?: boolean;
	/** The trace branch is not on this machine, so there is nothing to show. */
	missingWorktree?: boolean;
	/** Commits the local default branch is behind origin by; merging may conflict. */
	originBehindCount?: number;
	delivery?: string;
	prTitle?: string;
	prBody?: string;
	pullRequest?: PullRequest;
}

/**
 * One line-anchored review note, as `console.ReviewCommentInput` reads it. A
 * single-line comment omits `endLine` and an empty one omits `snippet`, because
 * the bee's rework prompt quotes the range and a zero-width range reads as a typo.
 */
export interface ReviewComment {
	path: string;
	side: 'old' | 'new';
	startLine: number;
	endLine?: number;
	snippet?: string;
	body: string;
}

/** Mirrors `console.ApproveTaskRequest`; only the delivery fields are sent when set. */
export interface ApproveTaskRequest {
	summary?: string;
	mergeMessage?: string;
	prTitle?: string;
	prBody?: string;
	draft?: boolean;
	runHooks?: boolean;
}

/** Mirrors `console.ApproveTaskResponse`. */
export interface ApproveTaskResult {
	traceId: string;
	taskId: string;
	commitSha?: string;
	prUrl?: string;
	prState?: string;
	published?: boolean;
	message?: string;
}

/**
 * Mirrors `console.RejectTaskRequest`. `headSha` pins the rejection to the commit
 * the reviewer read, and `comments` carries the annotated packet the bee is
 * reworked from; a plain rejection sends only the feedback.
 */
export interface RejectTaskRequest {
	feedback: string;
	headSha?: string;
	comments?: ReviewComment[];
}

/** Mirrors `console.RejectTaskResponse`; a rework task is created for the bee. */
export interface RejectTaskResult {
	traceId: string;
	taskId: string;
	reworkTaskId?: string;
	message?: string;
}

/**
 * The `scope` query `GET /api/bees` understands. `launchable` is the server's
 * default and the launch picker's shape; `colony` is every bee, script bees
 * included, and is the only scope that carries a last run.
 */
export type BeeScope = 'launchable' | 'colony';

/** Mirrors `console.BeeRunRef`, a bee's most recent headless run. */
export interface BeeRunRef {
	traceId: string;
	agentId: string;
	intent?: string;
	state: string;
	startedAt: string;
	finishedAt?: string;
}

/**
 * Mirrors `console.BeeView`. `GET /api/bees` returns the launchable bees by
 * default — the forms offer only what can be started — and the whole roster
 * under `?scope=colony`, which is what the Bees route reads.
 */
export interface Bee {
	role: string;
	adapter: string;
	promptTemplate: string;
	/** From bee YAML; absent when the bee declares no default sector. */
	sector?: string;
	worktree: boolean;
	/**
	 * `null`, not `[]`, for a bee whose prompt templates yield no intents:
	 * `console.BeeView.Intents` carries no `omitempty`, so Go marshals the nil slice
	 * that `prompts.DiscoverIntents` returns as `null`. Read it through
	 * `beeIntents`, which is the one place that knows.
	 */
	intents: string[] | null;
	defaultIntent?: string;
	/**
	 * Whether the effective adapter supports an interactive session. Absent from
	 * the launch picker, where every row is `true` by construction.
	 */
	interactive?: boolean;
	/**
	 * The newest run this bee ever had, or absent when it has never run one. Only
	 * the colony scope fills it, and it is server-derived on purpose: a client
	 * grouping a recent-run page would report a quiet bee as never-run.
	 */
	lastRun?: BeeRunRef;
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

/** Any JSON value; an event payload is author-supplied, so nothing narrower is true. */
export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

/** Mirrors `protocol.Event`, the raw envelope the feed carries beside its projection. */
export interface ProtocolEvent {
	protocolVersion: string;
	traceId: string;
	agentId: string;
	seq: number;
	type: string;
	createdAt: string;
	payload?: JsonValue;
}

/** How a subscription triggers a bee: a dispatched run, or a direct call. */
export type DispatchMode = 'task' | 'direct';

/** Mirrors `colony.TopologyBee`: one bee node with its prompt vocabulary. */
export interface TopologyBee {
	role: string;
	adapter: string;
	intents?: string[];
	defaultIntent?: string;
}

/** Mirrors `colony.TopologyEvent`: one bus event kind an edge references. */
export interface TopologyEvent {
	type: string;
	kind?: string;
	id: string;
}

/** Mirrors `colony.TopologyEdge`: a subscribe, publish, or invite rule. */
export interface TopologyEdge {
	kind: 'subscribe' | 'publish' | 'invite';
	from: string;
	to?: string;
	dispatch?: DispatchMode;
	/** The bee declared no `subscribes`, so any `task.ready` reaches it. */
	implicit?: boolean;
	/** Prompt vocabulary the invite hands the invited bee. */
	intent?: string;
	match?: Record<string, string>;
	/** An invite chained off another bee's output rather than a fixed role. */
	beeFrom?: string;
}

/** Mirrors `colony.Topology`, the config-derived EDA projection. */
export interface Topology {
	bees: TopologyBee[];
	events: TopologyEvent[];
	edges: TopologyEdge[];
	/** The same graph as Mermaid, identical to `paseka colony topology`. */
	mermaid?: string;
}

/** Mirrors `hiveview.EventLink`: the resource an event points at, when it names one. */
export interface EventLink {
	kind: string;
	traceId?: string;
	agentId?: string;
	taskId?: string;
	sessionId?: string;
}

/** Mirrors `hiveview.EventFeedItem`. */
export interface EventFeedItem extends SignalSummary {
	id: string;
	type: string;
	taskId?: string;
	/** Present on every feed row; the per-event raw view is this, not a second request. */
	raw: ProtocolEvent;
	/** Unused by the feed today: it names a related resource whose route is still a placeholder. */
	link?: EventLink;
}

/** Mirrors `hiveview.EventFeedPage`, one cursor-paginated page of the feed. */
export interface EventFeedPage {
	items: EventFeedItem[];
	nextCursor?: string;
	hasMore: boolean;
}

/** The four choreographed contracts, in the order the reference documents them. */
export const eventTypes = ['SIGNAL', 'INSIGHT', 'MUTATION', 'VERIFICATION'] as const;
export type EventType = (typeof eventTypes)[number];

/** A feed filter. Every field is optional because the colony-wide feed sets none. */
export interface EventFilters {
	traceId?: string;
	taskId?: string;
	bee?: string;
	type?: string;
	kind?: string;
	severity?: string;
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
