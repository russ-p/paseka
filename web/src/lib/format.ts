import type {
	AgentItem,
	ArtifactView,
	DashboardSummary,
	EventFilters,
	GitActionResult,
	GitBranch,
	GitPlaque,
	GitView,
	GitWorktree,
	HoneyReserve,
	HostStatus,
	InsightHighlight,
	JsonValue,
	NATSStatusView,
	ProtocolEvent,
	RunSummary,
	RuntimeStatus,
	SignalSummary,
	TaskSummary,
	Topology,
	TopologyEdge,
	TraceSummary,
	Usage,
	UsageAggregate
} from '$lib/api/types';
import type { StatusIconGlyph } from '$lib/components/StatusIcon.svelte';

/** One label/value line of a `MetaList`; the shape stays plain so it is unit-testable. */
export interface MetaRow {
	label: string;
	value: string;
	/** Render the value in a monospace face: paths, refs, SHAs. */
	mono?: boolean;
	/** Full text for the `Hint` popover when the line truncates. */
	hint?: string[];
	/** Render the value as an external link. */
	href?: string;
	/** Offer a copy button for the value — an id the operator pastes elsewhere. */
	copy?: boolean;
}

export function formatBytes(value: number | undefined): string | null {
	if (value === undefined || !Number.isFinite(value)) return null;
	const gib = value / 1024 ** 3;
	if (gib >= 100) return `${gib.toFixed(0)} GiB`;
	if (gib >= 10) return `${gib.toFixed(1)} GiB`;
	return `${gib.toFixed(2)} GiB`;
}

export function formatPercent(value: number | undefined): string | null {
	if (value === undefined || !Number.isFinite(value)) return null;
	return `${value.toFixed(0)}%`;
}

export function formatUptime(seconds: number | undefined): string | null {
	if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return null;
	const total = Math.floor(seconds);
	const days = Math.floor(total / 86400);
	const hours = Math.floor((total % 86400) / 3600);
	const minutes = Math.floor((total % 3600) / 60);
	if (days > 0) return `${days}d ${hours}h`;
	if (hours > 0) return `${hours}h ${minutes}m`;
	return `${minutes}m`;
}

/** 24-hour clock regardless of the browser locale, seconds included. */
export function formatClock(iso: string | undefined): string | null {
	if (!iso) return null;
	const parsed = new Date(iso);
	if (Number.isNaN(parsed.getTime())) return null;
	return parsed.toLocaleTimeString([], {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hourCycle: 'h23'
	});
}

export function formatFetchAge(seconds: number | undefined): string | null {
	if (seconds === undefined || !Number.isFinite(seconds) || seconds < 0) return null;
	if (seconds < 60) return 'just now';
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	return `${Math.floor(hours / 24)}d ago`;
}

export function runtimeMeta(runtime: RuntimeStatus | null): string {
	if (!runtime) return 'Not running';
	const parts: string[] = [];
	if (runtime.pid) parts.push(`pid ${runtime.pid}`);
	const started = formatClock(runtime.startedAt);
	if (started) parts.push(`started ${started}`);
	const heartbeat = formatClock(runtime.lastHeartbeatAt);
	if (heartbeat) parts.push(`heartbeat ${heartbeat}`);
	if (parts.length > 0) return parts.join(' · ');
	return runtime.alive ? 'Running' : 'Not running';
}

export type RuntimeActionKind = 'start' | 'stop' | 'busy' | 'choose';

/**
 * One runtime action per state. `stopping` blocks everything because
 * `Supervisor.Start` only skips spawning for `alive && running`; calling it
 * mid-shutdown would start a second runtime.
 */
export function runtimeAction(runtime: RuntimeStatus | null, busy: boolean): RuntimeActionKind {
	if (busy || runtime?.status === 'stopping') return 'busy';
	const status = runtime?.status ?? 'unknown';
	if (status === 'running') return 'stop';
	if (status === 'stopped' || status === 'stale') return 'start';
	return 'choose';
}

export function runtimeActionLabel(kind: RuntimeActionKind, status: string, action: string | null): string {
	switch (kind) {
		case 'start':
			return 'Start hive runtime';
		case 'stop':
			return 'Stop hive runtime';
		case 'busy':
			return action === 'starting' ? 'Starting hive runtime' : 'Stopping hive runtime';
		case 'choose':
			return `Hive runtime is ${status} — choose an action`;
	}
}

export function runtimeActionGlyph(kind: RuntimeActionKind): StatusIconGlyph {
	switch (kind) {
		case 'start':
			return 'play';
		case 'stop':
			return 'stop';
		case 'busy':
			return 'pending';
		case 'choose':
			return 'unknown';
	}
}

/** Heartbeat first: it is the liveness signal, pid and start time are context. */
export function runtimeDetailLines(runtime: RuntimeStatus | null): string[] {
	if (!runtime) return [];
	const lines: string[] = [];
	const heartbeat = formatClock(runtime.lastHeartbeatAt);
	if (heartbeat) lines.push(`heartbeat ${heartbeat}`);
	if (runtime.pid) lines.push(`pid ${runtime.pid}`);
	const started = formatClock(runtime.startedAt);
	if (started) lines.push(`started ${started}`);
	return lines;
}

export function runtimeDetail(runtime: RuntimeStatus | null): string {
	return runtimeDetailLines(runtime).join(' · ');
}

const runtimeHints: Record<string, string> = {
	starting: 'coming up',
	stopping: 'shutting down',
	stale: 'registry entry, start respawns',
	degraded: 'hive reported an error'
};

/** Status word plus a hint for the unusual values; the pid/heartbeat detail is its own line. */
export function runtimeStateNote(runtime: RuntimeStatus | null): string {
	const status = runtime?.status ?? 'unknown';
	return [status, runtimeHints[status]].filter(Boolean).join(' · ');
}

export function agentsMeta(agents: { count: number; afk: number; sessions: number } | null): string {
	if (!agents || agents.count === 0) return 'None active';
	const parts: string[] = [`${agents.count} live`];
	if (agents.afk > 0) parts.push(plural(agents.afk, 'afk'));
	if (agents.sessions > 0) parts.push(plural(agents.sessions, 'session'));
	return parts.join(' · ');
}

function agentLabels(items: AgentItem[]): string[] {
	return items.map((item) => `${item.kind}:${item.bee}`);
}

/** The clipped line: at most three bees. */
export function agentsDetail(items: AgentItem[] | undefined): string {
	if (!items || items.length === 0) return '';
	return agentLabels(items)
		.slice(0, 3)
		.join(' · ');
}

/** Every bee, for the hint of a clipped line. */
export function agentsDetailFull(items: AgentItem[] | undefined): string {
	if (!items || items.length === 0) return '';
	return agentLabels(items).join(' · ');
}

/** Hint lines for the host memory line: hostname and load when known. */
export function hostDetailLines(host: HostStatus | null, error: string): string[] {
	if (!host) return [error || 'Unavailable'];
	const lines: string[] = [];
	if (host.hostname) lines.push(host.hostname);
	if (host.os) lines.push(`${host.os} ${host.arch} · ${host.cpus} cpu`);
	lines.push(hostMeta(host, error));
	const load = hostLoad(host);
	if (load) lines.push(load);
	return lines;
}

export function hostBadge(host: HostStatus | null): string {
	const cpu = formatPercent(host?.cpuPercent);
	return cpu ?? '—';
}

export function hostMeta(host: HostStatus | null, error: string): string {
	if (!host) return error || 'Unavailable';
	const used = formatBytes(host.memUsedBytes);
	const total = formatBytes(host.memTotalBytes);
	if (used && total) return `${used.replace(' GiB', '')} / ${total}`;
	return host.error || 'Memory unavailable';
}

export function hostLoad(host: HostStatus | null): string {
	if (!host || host.load1 === undefined) return '';
	return `load ${host.load1.toFixed(2)}`;
}

export function hostDetail(host: HostStatus | null): string {
	if (!host) return '';
	const parts: string[] = [];
	if (host.hostname) parts.push(host.hostname);
	const load = hostLoad(host);
	if (load) parts.push(load);
	return parts.join(' · ');
}

export function gitSyncLabel(git: GitPlaque | null): string {
	if (!git) return '—';
	if (!git.originUrl) return 'no origin';
	if (git.ahead === undefined && git.behind === undefined) {
		const bits: string[] = [];
		if (git.note && !git.dirty) bits.push('fetch');
		if (git.dirty) bits.push('dirty');
		return bits.join(' ') || git.defaultBranch || 'git';
	}
	if ((git.ahead ?? 0) > 0 && (git.behind ?? 0) > 0) return `↑${git.ahead} ↓${git.behind}`;
	if ((git.ahead ?? 0) > 0) return `↑${git.ahead}`;
	if ((git.behind ?? 0) > 0) return `↓${git.behind}`;
	return 'in sync';
}

export function gitNeedsAttention(git: GitPlaque | null): boolean {
	if (!git) return true;
	return !git.originUrl || git.dirty || (git.ahead ?? 0) > 0 || (git.behind ?? 0) > 0;
}

/** Hint lines for the git branch line: origin and divergence are not on screen. */
export function gitLines(git: GitPlaque | null, error: string): string[] {
	if (!git) return [error || 'Unavailable'];
	const lines = [gitMeta(git, error)];
	lines.push(git.originUrl ? `origin ${git.originUrl.replace(/\.git$/, '')}` : 'no origin');
	const sync = gitSyncLabel(git);
	if (sync !== 'in sync' && sync !== '—') lines.push(sync);
	if (git.dirty) lines.push('uncommitted changes');
	return lines;
}

export function gitMeta(git: GitPlaque | null, error: string): string {
	if (!git) return error || 'Unavailable';
	return git.defaultBranch || git.branch || '—';
}

export function gitDetail(git: GitPlaque | null, error: string): string {
	if (!git) return '';
	const parts: string[] = [];
	if (git.headShaShort) parts.push(git.headShaShort);
	const age = formatFetchAge(git.lastFetchAgeSeconds);
	if (age) parts.push(`fetch ${age}`);
	if (error) parts.push(error);
	return parts.join(' · ');
}

/**
 * How the clone stands against origin, in one word. The topbar's
 * `gitSyncLabel` falls back to the branch name when the remote-tracking refs are
 * missing, which would repeat the Default branch row sitting next to it; here
 * the missing comparison is said outright and the server's `Note` explains it.
 */
export function gitSyncWord(git: GitView | null): string {
	if (!git) return '—';
	if (!git.originUrl) return 'no origin';
	if (git.ahead === undefined && git.behind === undefined) {
		return git.dirty ? 'dirty · not compared' : 'not compared';
	}
	return gitSyncLabel(git);
}

/**
 * The checkout itself: where HEAD is, what is published, and whether the working
 * tree is dirty. Split from the origin half so a two-column block pairs a row
 * with a row about the same thing instead of an arbitrary neighbour.
 */
export function gitCloneRows(git: GitView | null): MetaRow[] {
	if (!git) return [];
	const rows: MetaRow[] = [];
	if (git.branch) {
		// The branch is what the operator types into `paseka` commands, so it copies.
		rows.push({ label: 'Branch', value: git.branch, mono: true, copy: true, hint: [git.branch] });
	}
	rows.push({
		label: 'HEAD',
		value: git.headShaShort || git.headSha || '—',
		mono: true,
		// The copy button takes width, so the short sha truncates first; the full one is the truth.
		hint: git.headSha && git.headSha !== git.headShaShort ? [git.headSha] : undefined
	});
	if (git.defaultBranch) {
		rows.push({ label: 'Default branch', value: git.defaultBranch, mono: true });
	}
	if (git.dirty) rows.push({ label: 'Working tree', value: 'uncommitted changes' });
	return rows;
}

/**
 * How the clone stands against origin. A row appears only when it says
 * something: no origin, no divergence, and no note leave the block quiet instead
 * of padding it with `—`. The divergence numbers live inside the sync word
 * (`↑3 ↓1`), so they are not repeated as rows of their own.
 */
export function gitOriginRows(git: GitView | null): MetaRow[] {
	if (!git) return [];
	const rows: MetaRow[] = [{ label: 'Sync', value: gitSyncWord(git) }];
	if (git.originUrl) {
		// An operator rewrites this into a remote, so it copies.
		rows.push({ label: 'Origin', value: git.originUrl, mono: true, copy: true, hint: [git.originUrl] });
	}
	const fetched = formatFetchAge(git.lastFetchAgeSeconds);
	if (fetched) rows.push({ label: 'Last fetch', value: fetched });
	// The server sets this when the comparison is impossible; without it an empty
	// unpublished list looks like there is nothing to push.
	if (git.note) rows.push({ label: 'Note', value: git.note });
	return rows;
}

/**
 * One word per branch, naming the thing that decides what happens next. Order is
 * the operator's: you cannot delete the branch you are standing on, a leftover is
 * the sweep target, and a merged branch is merely tidyable.
 */
export function gitBranchState(branch: GitBranch): { status: string; label: string } | null {
	if (branch.current) return { status: 'active', label: 'current' };
	if (branch.leftover) return { status: 'leftover', label: 'leftover' };
	if (branch.merged) return { status: 'merged', label: 'merged' };
	return null;
}

/** The facts that are not state: which branch gets published, which holds a checkout. */
export function gitBranchFlags(branch: GitBranch): string {
	const flags: string[] = [];
	if (branch.default) flags.push('default');
	if (branch.worktreePath) flags.push('worktree');
	return flags.join(' · ');
}

export function gitWorktreeState(worktree: GitWorktree): { status: string; label: string } {
	return worktree.dirty
		? { status: 'dirty', label: 'dirty' }
		: { status: 'clean', label: 'clean' };
}

/** The branches the maintenance sweep is allowed to delete. */
export function gitLeftoverNames(branches: GitBranch[] | undefined): string[] {
	return (branches ?? []).filter((branch) => branch.leftover).map((branch) => branch.name);
}

/**
 * What a git POST actually did. `message` is whatever git printed and is often
 * empty, so each call site passes the word it would have used; a batch delete
 * reports per name instead of a single message.
 */
export function gitActionMessage(result: GitActionResult | null, fallback: string): string {
	if (!result) return fallback;
	if (result.message) return result.message;
	const items = result.results ?? [];
	const failed = items.filter((item) => !item.ok);
	if (failed.length > 0) {
		return failed.map((item) => `${item.name}: ${item.error || 'failed'}`).join(' · ');
	}
	if (items.length > 0) return `${items.length} deleted`;
	return fallback;
}

/** The five things an operator can ask of the clone, and the only mutations the page offers. */
export type GitAction = 'fetch' | 'push' | 'pull' | 'prune' | 'delete';

/** Button label: the verb while idle, the present participle while in flight. */
export function gitActionLabel(action: GitAction, busy: boolean): string {
	const verbs: Record<GitAction, [idle: string, pending: string]> = {
		fetch: ['Fetch', 'Fetching…'],
		push: ['Push', 'Pushing…'],
		pull: ['Pull', 'Pulling…'],
		prune: ['Prune orphans', 'Pruning…'],
		delete: ['Delete leftovers', 'Deleting…']
	};
	const [idle, pending] = verbs[action];
	return busy ? pending : idle;
}

/** Timestamp for a row: `2026-09-25 18:04:22`, locale-independent, 24-hour. */
export function formatTimestamp(iso: string | undefined): string {
	if (!iso) return '—';
	const parsed = new Date(iso);
	if (Number.isNaN(parsed.getTime())) return '—';
	const pad = (value: number) => String(value).padStart(2, '0');
	return [
		parsed.getFullYear(),
		pad(parsed.getMonth() + 1),
		pad(parsed.getDate())
	].join('-') + ` ${pad(parsed.getHours())}:${pad(parsed.getMinutes())}:${pad(parsed.getSeconds())}`;
}

/** NATS report to a transport status. `idle` means NATS is simply not configured. */
export function natsStatus(nats: NATSStatusView | undefined): string {
	if (!nats) return 'unknown';
	if (!nats.configured) return 'idle';
	return nats.connected ? 'connected' : 'disconnected';
}

/** The same status as an operator-facing word, so `idle` does not leak into the UI. */
export function natsLabel(status: string): string {
	switch (status) {
		case 'connected':
			return 'connected';
		case 'disconnected':
			return 'disconnected';
		case 'idle':
			return 'not configured';
		default:
			return 'unknown';
	}
}

/** Task counts sorted by status so the tile reads the same way every poll. */
export function taskCountEntries(counts: Record<string, number> | undefined): Array<[string, number]> {
	return Object.entries(counts ?? {}).sort(([a], [b]) => a.localeCompare(b));
}

/** `1 run` but `4 runs`; the count is always right of the noun. */function plural(count: number, noun: string): string {
	return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

/** A trace reads as active, failed, or idle; the raw status never invents a new word. */
export function traceState(trace: TraceSummary): 'active' | 'failed' | 'idle' {
	if (trace.hasActive) return 'active';
	if (trace.hasFailures) return 'failed';
	return 'idle';
}

export function traceStateLabel(trace: TraceSummary): string {
	switch (traceState(trace)) {
		case 'active':
			return 'active';
		case 'failed':
			return 'failures';
		default:
			return plural(trace.runCount, 'run');
	}
}

export function tracePrimaryLabel(trace: TraceSummary): string {
	return trace.title || trace.traceId;
}

export function traceMeta(trace: TraceSummary): string {
	const parts: string[] = [plural(trace.runCount, 'run'), plural(trace.taskCount, 'task')];
	const at = formatTimestamp(trace.lastActivityAt);
	if (at !== '—') parts.push(at);
	return parts.join(' · ');
}

export function runStateLabel(state: string): string {
	return state || 'unknown';
}

/** A failed or cancelled run is why it is on the dashboard. */
export function runNeedsAttention(run: RunSummary): boolean {
	const state = run.state.trim().toLowerCase();
	return state === 'failed' || state === 'cancelled' || state === 'killed';
}

export function insightTime(insight: InsightHighlight): string {
	return formatTimestamp(insight.createdAt);
}

export function dashboardFailedRuns(dashboard: DashboardSummary | null): RunSummary[] {
	return (dashboard?.failedRuns ?? []).filter(runNeedsAttention);
}

/** `standing`, `active`, `failures` — the flags the trace summary carries, or nothing. */
export function traceFlags(trace: TraceSummary): string[] {
	const flags: string[] = [];
	if (trace.standing) flags.push('standing');
	if (trace.hasActive) flags.push('active');
	if (trace.hasFailures) flags.push('failures');
	return flags;
}

/** Bees joined for display; a trail with none has not picked a worker yet. */
export function traceBees(trace: TraceSummary): string {
	return trace.bees?.join(', ') || '—';
}

/**
 * What the honey bar is measured against. A standing trail is capped by its
 * stipend; any other trail by whatever the top-ups have added so far.
 */
export function energyDenominator(energy: HoneyReserve): number {
	const budget = energy.energyBudget ?? 0;
	if (energy.standing && budget > 0) return budget;
	const allocated = energy.energyAllocated ?? 0;
	return allocated > 0 ? allocated : budget;
}

export function energyLabel(energy: HoneyReserve): string {
	const denominator = energyDenominator(energy);
	if (denominator > 0) return `${energy.energyRemaining ?? 0} / ${denominator}`;
	return String(energy.energyRemaining ?? 0);
}

/** Provenance of the reserve: a standing stipend, or a seed plus top-ups. */
export function energyMetaLabel(energy: HoneyReserve): string {
	if (energy.standing) return `stipend ${energy.energyBudget ?? 0}`;
	if ((energy.energyAdded ?? 0) > 0) {
		return `seed ${energy.energyBudget ?? 0} · topped ${energy.energyAdded}`;
	}
	return '';
}

/** No reserve means NATS or the task ledger is not wired, not that the trail is empty. */
export function energyAvailable(energy: HoneyReserve): boolean {
	return energyDenominator(energy) > 0 || (energy.energyRemaining ?? 0) > 0;
}

export function energyLowLabel(energy: HoneyReserve): string | null {
	return energy.lowEnergy ? 'low' : null;
}

/** Compacts a token count: `940`, `1.2k`, `3.4M`. */
export function formatTokenCount(value: number | undefined): string {
	if (value === undefined || !Number.isFinite(value)) return '—';
	if (value < 1000) return String(value);
	if (value < 1_000_000) return `${(value / 1000).toFixed(1)}k`;
	return `${(value / 1_000_000).toFixed(1)}M`;
}

/** `840ms`, `12s`, `3m 5s`, `2h 10m`. */
export function formatDuration(ms: number | undefined): string {
	if (ms === undefined || !Number.isFinite(ms) || ms < 0) return '—';
	if (ms < 1000) return `${Math.round(ms)}ms`;
	const seconds = Math.floor(ms / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
	return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

/** The trace's token spend; zero counts stay visible because the trail reported them. */
export function usageRows(usage: UsageAggregate | undefined): MetaRow[] {
	if (!usage) return [];
	const rows: MetaRow[] = [
		{ label: 'Input tokens', value: formatTokenCount(usage.inputTokens) },
		{ label: 'Output tokens', value: formatTokenCount(usage.outputTokens) },
		{ label: 'Cache read', value: formatTokenCount(usage.cacheReadTokens) },
		{ label: 'Cache write', value: formatTokenCount(usage.cacheWriteTokens) }
	];
	if (usage.runCountWithUsage > 0) {
		rows.push({ label: 'Runs with usage', value: String(usage.runCountWithUsage) });
	}
	return rows;
}

/** One-line token spend for a run row, or an empty string when it reported none. */
export function runUsageLabel(usage: Usage | undefined): string {
	if (!usage) return '';
	const parts: string[] = [];
	if (usage.inputTokens > 0) parts.push(`${formatTokenCount(usage.inputTokens)} in`);
	if (usage.outputTokens > 0) parts.push(`${formatTokenCount(usage.outputTokens)} out`);
	const cache = (usage.cacheReadTokens ?? 0) + (usage.cacheWriteTokens ?? 0);
	if (cache > 0) parts.push(`${formatTokenCount(cache)} cache`);
	return parts.join(' · ');
}

/** A run's third line: when it started, what it spent, and how long it took. */
export function runMeta(run: RunSummary): string {
	const parts: string[] = [formatTimestamp(run.startedAt)];
	const usage = runUsageLabel(run.usage);
	if (usage) parts.push(usage);
	const duration = run.finishedAt
		? formatDuration(new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime())
		: '';
	if (duration && duration !== '—') parts.push(duration);
	return parts.join(' · ');
}

export function taskPrimaryLabel(task: TaskSummary): string {
	return task.title || task.taskId;
}

export function taskMeta(task: TaskSummary): string {
	return task.bee ? `${task.taskId} · ${task.bee}` : task.taskId;
}

/** An artifact's headline: the author's title, else its kind, else the raw ref. */
export function artifactLabel(artifact: Pick<ArtifactView, 'title' | 'artifactKind' | 'ref'>): string {
	return artifact.title || artifact.artifactKind || artifact.ref;
}

/** The artifact's second line: kind, producer, and the canonical comb path. */
export function artifactMeta(artifact: Pick<ArtifactView, 'artifactKind' | 'producer' | 'ref'>): string {
	const parts = [artifact.artifactKind];
	if (artifact.producer) parts.push(artifact.producer);
	parts.push(artifact.ref);
	return parts.join(' · ');
}

/** `announced` means the colony was told; `staged` means only the comb has it. */
export function artifactState(artifact: Pick<ArtifactView, 'announced'>): 'announced' | 'staged' {
	return artifact.announced ? 'announced' : 'staged';
}

/**
 * Byte counts in a process column are a different scale from machine memory:
 * most processes are MiB, so the GiB formatter the metric tiles use would print
 * `0.00 GiB` down the whole table and make the rows incomparable. The unit
 * follows the value here.
 */
export function formatProcessBytes(value: number | undefined): string | null {
	if (value === undefined || !Number.isFinite(value)) return null;
	const mib = value / 1024 ** 2;
	if (mib >= 10) return `${mib.toFixed(0)} MiB`;
	if (mib >= 1) return `${mib.toFixed(1)} MiB`;
	const kib = value / 1024;
	if (kib >= 1) return `${kib.toFixed(0)} KiB`;
	return `${Math.round(value)} B`;
}

/** `used / total`, or `null` when the server could not measure either half. */
function bytePairWord(used: number | undefined, total: number | undefined): string | null {
	const usedWord = formatBytes(used);
	const totalWord = formatBytes(total);
	if (!usedWord || !totalWord) return null;
	return `${usedWord} / ${totalWord}`;
}

export function systemMemoryWord(host: HostStatus | null): string | null {
	if (!host) return null;
	return bytePairWord(host.memUsedBytes, host.memTotalBytes);
}

export function systemDiskWord(host: HostStatus | null): string | null {
	if (!host) return null;
	return bytePairWord(host.diskUsedBytes, host.diskTotalBytes);
}

export function systemAvailableWord(host: HostStatus | null): string | null {
	return formatBytes(host?.memAvailableBytes);
}

/** The three averages together: a partial set is not a load number. */
export function systemLoadWord(host: HostStatus | null): string | null {
	if (!host || host.load1 === undefined || host.load5 === undefined || host.load15 === undefined) {
		return null;
	}
	return `${host.load1.toFixed(2)} / ${host.load5.toFixed(2)} / ${host.load15.toFixed(2)}`;
}

/**
 * CPU percent is a delta between two `/proc/stat` samples, so the first poll of
 * a fresh console cannot have one. That absence is expected, not a fault, and
 * the tile says so instead of pretending the box is idle.
 */
export function systemCpuPending(host: HostStatus | null): boolean {
	return host !== null && host.cpuPercent === undefined;
}

export const cpuPendingHint = [
	'CPU percent is the difference between two /proc/stat samples.',
	'It appears on the next poll.'
];

/** Which box this is. Every row is conditional, so a degraded snapshot is quiet rather than padded. */
export function systemIdentityRows(host: HostStatus | null): MetaRow[] {
	if (!host) return [];
	const rows: MetaRow[] = [];
	if (host.hostname) {
		// The hostname is the first thing pasted into a bug report or an ssh command.
		rows.push({ label: 'Hostname', value: host.hostname, mono: true, copy: true, hint: [host.hostname] });
	}
	if (host.kernel) rows.push({ label: 'Kernel', value: host.kernel, mono: true, hint: [host.kernel] });
	if (host.os || host.arch) {
		const platform = `${host.os || '—'} / ${host.arch || '—'}`;
		rows.push({ label: 'OS / arch', value: platform, hint: [platform] });
	}
	if (host.cpus) {
		// Without the cpu count a load average is an unreadable number.
		rows.push({ label: 'CPUs', value: String(host.cpus) });
	}
	const uptime = formatUptime(host.uptimeSeconds);
	if (uptime) rows.push({ label: 'Uptime', value: uptime });
	if (host.consolePid) {
		const pid = String(host.consolePid);
		rows.push({ label: 'Console PID', value: pid, mono: true, copy: true, hint: [pid] });
	}
	if (host.goVersion) {
		// The toolchain that built the running binary, for "it works on my box" reports.
		rows.push({ label: 'Go', value: host.goVersion, mono: true, copy: true, hint: [host.goVersion] });
	}
	return rows;
}

/**
 * The pids the Live bees panel reports, for marking adapter processes in the
 * table. A client-side join is the whole point: System Info never asks the
 * agents API whether a process is a bee, so a compiler or a test runner still
 * shows up here as ordinary load.
 */
export function liveBeePids(items: AgentItem[] | undefined): Set<number> {
	return new Set((items ?? []).map((item) => item.pid));
}

/**
 * The feed row's provenance line: the contract and the payload kind, which are
 * different things (`VERIFICATION` carrying `review.gate`) and are worth showing
 * together. When they happen to be the same word the line says it once, and an
 * absent half leaves the other alone rather than adding a dangling separator.
 */
export function eventKindLine(event: Pick<SignalSummary, 'type' | 'payloadKind'>): string {
	if (!event.payloadKind) return event.type ?? '';
	if (!event.type || event.type === event.payloadKind) return event.payloadKind;
	return `${event.type} · ${event.payloadKind}`;
}

/** How many filters narrow the feed, so the collapsed panel can say so. */
export function activeFilterCount(filters: EventFilters): number {
	return Object.values(filters).filter((value) => value && value.trim() !== '').length;
}

/** The active filters as one readable phrase for the collapsed panel's summary. */
export function eventFilterSummary(filters: EventFilters): string {
	const parts: string[] = [];
	if (filters.traceId) parts.push(filters.traceId);
	if (filters.taskId) parts.push(`task ${filters.taskId}`);
	if (filters.bee) parts.push(filters.bee);
	if (filters.type) parts.push(filters.type);
	if (filters.kind) parts.push(`kind ${filters.kind}`);
	if (filters.severity) parts.push(`severity ${filters.severity}`);
	return parts.join(' · ');
}

/** The counts the topology header leads with, in the order the legacy showed them. */
export function topologyCounts(topology: Topology | null): { label: string; value: string }[] {
	if (!topology) return [];
	return [
		{ label: 'Bees', value: String(topology.bees.length) },
		{ label: 'Event kinds', value: String(topology.events.length) },
		{ label: 'Rules', value: String(topology.edges.length) }
	];
}

/**
 * The readable fields of an event payload, in a stable order.
 *
 * A payload is most often flat scalars, but not always: `artifact.written`
 * announces `{"artifacts": [{artifactKind, ref, title}]}` and has no scalar at
 * the top level at all, so a scalars-only digest renders that row blank. An array
 * of objects therefore contributes its first element's readable fields, which is
 * enough to say what arrived without a per-kind lookup table that would need
 * updating every time the protocol grows a payload.
 */
function payloadFields(payload: JsonValue | undefined): [string, string, boolean][] {
	if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return [];
	const out: [string, string, boolean][] = [];
	for (const [key, value] of Object.entries(payload)) {
		if (key === 'kind') continue;
		if (isScalar(value)) {
			out.push([key, String(value), false]);
			continue;
		}
		const first = Array.isArray(value) ? value[0] : undefined;
		if (!first || typeof first !== 'object' || Array.isArray(first)) continue;
		const inner = Object.entries(first)
			.filter(([, item]) => isScalar(item))
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([, item]) => String(item));
		if (inner.length > 0) out.push([key, inner.join(' · '), true]);
	}
	return out.sort(([a], [b]) => a.localeCompare(b));
}

function isScalar(value: JsonValue | undefined): value is string | number | boolean {
	return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean';
}

/**
 * A readable line for an event payload that has no `summary`. Most payloads carry
 * one, but not all — `trace.title` carries a `title` — so the fallback lists the
 * payload's scalar fields rather than special-casing kinds, which would need
 * updating every time the protocol grows one.
 */
export function payloadDigest(payload: JsonValue | undefined, limit = 3): string {
	const fields = payloadFields(payload);
	if (fields.length === 0) return '';
	const shown = fields
		.slice(0, limit)
		.map(([key, value, fromList]) => `${key}${fromList ? ':' : '='}${value}`);
	const rest = fields.length - shown.length;
	return `${shown.join(' · ')}${rest > 0 ? ` · +${rest}` : ''}`;
}

/**
 * A recorded run event as a feed row, so the run detail reuses `SignalCard` — the
 * same component the Dashboard and the Timeline feed — instead of growing a
 * second event row. The projection is derived here rather than asked for twice:
 * the run's events endpoint hands back raw envelopes, and the fields a human reads
 * are a handful of keys inside each payload.
 */
export function runEventSummary(event: ProtocolEvent): SignalSummary {
	const payload = event.payload;
	const object = payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : undefined;
	const pick = (key: string): string | undefined => {
		const value = object?.[key];
		return isScalar(value) && String(value).trim() !== '' ? String(value) : undefined;
	};
	return {
		createdAt: event.createdAt,
		traceId: event.traceId,
		agentId: event.agentId,
		type: event.type,
		payloadKind: pick('kind'),
		severity: pick('severity'),
		// A payload without a summary still says something, so the digest stands in
		// rather than the row rendering blank.
		summary: pick('summary') ?? payloadDigest(payload) ?? ''
	};
}

/** How long a run took, or `null` while it is still going. */
export function runDurationMs(run: Pick<RunSummary, 'startedAt' | 'finishedAt'>): number | null {
	if (!run.finishedAt) return null;
	const started = Date.parse(run.startedAt);
	const finished = Date.parse(run.finishedAt);
	if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) return null;
	return finished - started;
}

/**
 * What a run is, as `MetaList` rows. Every row is conditional: a run that never
 * reported usage, an intent, or a provider session leaves those out rather than
 * padding the block with dashes.
 */
export function runIdentityRows(
	run: RunSummary | null,
	/** Where a trail id leads. The base path is the caller's, not a formatter's. */
	trailHref?: (traceId: string) => string
): MetaRow[] {
	if (!run) return [];
	const rows: MetaRow[] = [];
	rows.push({
		label: 'Trail',
		value: run.traceId,
		mono: true,
		href: trailHref?.(run.traceId)
	});
	rows.push({ label: 'Agent', value: run.agentId, mono: true, copy: true, hint: [run.agentId] });
	rows.push({ label: 'Bee', value: run.bee, mono: true });
	rows.push({ label: 'Adapter', value: run.adapter, mono: true });
	if (run.taskId) rows.push({ label: 'Task', value: run.taskId, mono: true });
	if (run.intent) rows.push({ label: 'Intent', value: run.intent, mono: true });
	rows.push({
		label: 'Started',
		value: formatTimestamp(run.startedAt),
		hint: [formatTimestamp(run.startedAt), run.startedAt]
	});
	if (run.finishedAt) {
		rows.push({
			label: 'Finished',
			value: formatTimestamp(run.finishedAt),
			hint: [formatTimestamp(run.finishedAt), run.finishedAt]
		});
	}
	const duration = runDurationMs(run);
	if (duration !== null) {
		rows.push({ label: 'Duration', value: formatDuration(duration) });
	} else if (run.state === 'running') {
		// A run still going has no finish line, so the row says so rather than
		// reporting a duration of zero.
		rows.push({ label: 'Duration', value: 'running' });
	}
	if (run.providerSessionId) {
		rows.push({
			label: 'Provider session',
			value: run.providerSessionId,
			mono: true,
			copy: true,
			hint: [run.providerSessionId]
		});
	}
	rows.push({ label: 'Workspace', value: run.workspace, mono: true, hint: [run.workspace] });
	// The run directory is where the prompt, events, and meta live, so it is the one
	// path an operator is likely to open a terminal on.
	rows.push({ label: 'Run dir', value: run.runDir, mono: true, copy: true, hint: [run.runDir] });
	return rows;
}

/** One run's token spend as rows, reusing the trail's token formatters. */
export function runUsageRows(usage: Usage | undefined): MetaRow[] {
	if (!usage) return [];
	const rows: MetaRow[] = [
		{ label: 'Input tokens', value: formatTokenCount(usage.inputTokens) },
		{ label: 'Output tokens', value: formatTokenCount(usage.outputTokens) }
	];
	const cacheRead = usage.cacheReadTokens ?? 0;
	const cacheWrite = usage.cacheWriteTokens ?? 0;
	if (cacheRead > 0) rows.push({ label: 'Cache read', value: formatTokenCount(cacheRead) });
	if (cacheWrite > 0) rows.push({ label: 'Cache write', value: formatTokenCount(cacheWrite) });
	if (usage.source) rows.push({ label: 'Counted by', value: usage.source });
	return rows;
}
