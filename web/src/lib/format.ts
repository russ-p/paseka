import type { AgentItem, GitPlaque, HostStatus, RuntimeStatus } from '$lib/api/types';
import type { StatusIconGlyph } from '$lib/components/StatusIcon.svelte';

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
	if (agents.afk > 0) parts.push(`${agents.afk} afk`);
	if (agents.sessions > 0) parts.push(`${agents.sessions} session`);
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
