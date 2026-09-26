import { describe, expect, it } from 'vitest';
import type { GitPlaque, HostStatus, RuntimeStatus } from '$lib/api/types';
import { dashboardSummary, insightHighlight, runSummary, traceSummary } from '../tests/fixtures';
import {
	agentsDetail,
	agentsDetailFull,
	agentsMeta,
	dashboardFailedRuns,
	formatTimestamp,
	insightTime,
	natsLabel,
	natsStatus,
	runNeedsAttention,
	runStateLabel,
	runtimeAction,
	runtimeActionGlyph,
	runtimeActionLabel,
	runtimeDetail,
	runtimeStateNote,
	formatBytes,
	formatClock,
	formatFetchAge,
	formatPercent,
	formatUptime,
	gitDetail,
	gitMeta,
	gitNeedsAttention,
	gitSyncLabel,
	hostBadge,
	hostDetail,
	hostLoad,
	hostMeta,
	runtimeMeta,
	taskCountEntries,
	traceMeta,
	tracePrimaryLabel,
	traceState,
	traceStateLabel
} from './format';

const running: RuntimeStatus = { status: 'running', alive: true, pid: 42, startedAt: '2026-09-25T10:00:00Z' };
const stopped: RuntimeStatus = { status: 'stopped', alive: false };

const host: HostStatus = {
	os: 'linux',
	arch: 'amd64',
	cpus: 16,
	hostname: 'thinkpad',
	load1: 1.25,
	cpuPercent: 13.4,
	memUsedBytes: 12_768_492_032,
	memTotalBytes: 33_404_239_232
};

const git: GitPlaque = {
	branch: 'paseka/trace-1',
	headSha: 'ba7b43c0d1',
	headShaShort: 'ba7b43c',
	dirty: true,
	defaultBranch: 'main',
	originUrl: 'git@github.com:example/paseka.git',
	ahead: 3,
	behind: 0,
	lastFetchAgeSeconds: 432_000
};

describe('topbar formatting', () => {
	it('formats byte, percent, uptime, clock, and fetch age values', () => {
		expect(formatBytes(33_404_239_232)).toBe('31.1 GiB');
		expect(formatBytes(undefined)).toBeNull();
		expect(formatPercent(13.4)).toBe('13%');
		expect(formatPercent(undefined)).toBeNull();
		expect(formatUptime(94_497)).toBe('1d 2h');
		expect(formatUptime(3_900)).toBe('1h 5m');
		expect(formatUptime(undefined)).toBeNull();
		expect(formatClock('2026-09-25T10:00:00Z')).toMatch(/\d{2}:\d{2}/);
		expect(formatClock('not-a-date')).toBeNull();
		expect(formatFetchAge(30)).toBe('just now');
		expect(formatFetchAge(432_000)).toBe('5d ago');
		expect(formatFetchAge(undefined)).toBeNull();
	});

	it('summarises the runtime and gates the start and stop actions', () => {
		expect(runtimeMeta(running)).toContain('pid 42');
		expect(runtimeMeta(stopped)).toBe('Not running');
		expect(runtimeMeta(null)).toBe('Not running');
	});

	it('maps every runtime status to exactly one action', () => {
		expect(runtimeAction(stopped, false)).toBe('start');
		expect(runtimeAction({ status: 'stale', alive: false }, false)).toBe('start');
		expect(runtimeAction(running, false)).toBe('stop');
		expect(runtimeAction({ status: 'stopping', alive: true }, false)).toBe('busy');
		expect(runtimeAction({ status: 'starting', alive: false }, false)).toBe('choose');
		expect(runtimeAction({ status: 'degraded', alive: true }, false)).toBe('choose');
		expect(runtimeAction(null, false)).toBe('choose');
	});

	it('blocks actions while a mutation is in flight', () => {
		expect(runtimeAction(stopped, true)).toBe('busy');
		expect(runtimeAction(running, true)).toBe('busy');
	});

	it('labels and pictures each action', () => {
		expect(runtimeActionLabel('start', 'stopped', null)).toBe('Start hive runtime');
		expect(runtimeActionLabel('stop', 'running', null)).toBe('Stop hive runtime');
		expect(runtimeActionLabel('busy', 'stopped', 'starting')).toBe('Starting hive runtime');
		expect(runtimeActionLabel('busy', 'stopping', null)).toBe('Stopping hive runtime');
		expect(runtimeActionLabel('choose', 'degraded', null)).toContain('degraded');

		expect(runtimeActionGlyph('start')).toBe('play');
		expect(runtimeActionGlyph('stop')).toBe('stop');
		expect(runtimeActionGlyph('busy')).toBe('pending');
		expect(runtimeActionGlyph('choose')).toBe('unknown');
	});

	it('names every runtime status in the text line', () => {
		expect(runtimeStateNote({ status: 'stale', alive: false })).toBe('stale · registry entry, start respawns');
		expect(runtimeStateNote({ status: 'stopping', alive: true })).toBe('stopping · shutting down');
		expect(runtimeStateNote({ status: 'starting', alive: false })).toBe('starting · coming up');
		expect(runtimeStateNote({ status: 'degraded', alive: true })).toBe('degraded · hive reported an error');
		expect(runtimeStateNote({ status: 'weird', alive: false })).toBe('weird');
		expect(runtimeStateNote(null)).toBe('unknown');
		expect(runtimeStateNote(stopped)).toBe('stopped');
		expect(runtimeStateNote(running)).toBe('running');
		const detailed = runtimeDetail({
			status: 'running',
			alive: true,
			pid: 9,
			startedAt: '2026-09-25T10:00:00Z',
			lastHeartbeatAt: '2026-09-25T10:05:00Z'
		});
		expect(detailed).toContain('pid 9 · started');
		expect(detailed).toContain('heartbeat');
		expect(runtimeDetail({ status: 'running', alive: true, startedAt: 'nope' })).toBe('');
	});

	it('keeps the live detail separate from the status word', () => {
		expect(runtimeDetail(null)).toBe('');
		expect(runtimeDetail(stopped)).toBe('');
		expect(runtimeDetail({ status: 'running', alive: true, pid: 42 })).toBe('pid 42');
	});

	it('summarises live bees from the agents projection', () => {
		expect(agentsMeta(null)).toBe('None active');
		expect(agentsMeta({ count: 0, afk: 0, sessions: 0 })).toBe('None active');
		expect(agentsMeta({ count: 3, afk: 1, sessions: 2 })).toBe('3 live · 1 afk · 2 sessions');
		expect(agentsMeta({ count: 4, afk: 2, sessions: 1 })).toBe('4 live · 2 afks · 1 session');
		expect(
			agentsDetail([
				{ kind: 'afk', bee: 'builder', pid: 1, traceId: 't1', agentId: 'a1', startedAt: '', runDir: '' },
				{ kind: 'session', bee: 'guard', pid: 2, traceId: 't2', agentId: 'a2', startedAt: '', runDir: '' }
			])
		).toBe('afk:builder · session:guard');
		expect(agentsDetail(undefined)).toBe('');
		expect(agentsDetailFull(undefined)).toBe('');

		const many = ['afk:builder', 'session:guard', 'afk:drone', 'afk:scout'].map((label, index) => ({
			kind: label.split(':')[0],
			bee: label.split(':')[1],
			pid: index,
			traceId: `t${index}`,
			agentId: `a${index}`,
			startedAt: '',
			runDir: ''
		}));
		expect(agentsDetail(many)).toBe('afk:builder · session:guard · afk:drone');
		expect(agentsDetailFull(many)).toBe('afk:builder · session:guard · afk:drone · afk:scout');
	});

	it('summarises host load and memory', () => {
		expect(hostBadge(host)).toBe('13%');
		expect(hostBadge(null)).toBe('—');
		expect(hostMeta(host, '')).toBe('11.9 / 31.1 GiB');
		expect(hostMeta(null, 'collect failed')).toBe('collect failed');
		expect(hostMeta(null, '')).toBe('Unavailable');
		expect(hostLoad(host)).toBe('load 1.25');
		expect(hostLoad(null)).toBe('');
		expect(hostDetail(host)).toBe('thinkpad · load 1.25');
		expect(hostDetail(null)).toBe('');
	});

	it('summarises git sync state and marks attention', () => {
		expect(gitSyncLabel(git)).toBe('↑3');
		expect(gitNeedsAttention(git)).toBe(true);
		expect(gitMeta(git, '')).toBe('main');
		expect(gitDetail(git, '')).toContain('ba7b43c');
		expect(gitDetail(git, 'fetch failed')).toContain('fetch failed');

		const clean: GitPlaque = { ...git, dirty: false, ahead: 0, behind: 0, lastFetchAgeSeconds: 10 };
		expect(gitSyncLabel(clean)).toBe('in sync');
		expect(gitNeedsAttention(clean)).toBe(false);

		const noOrigin: GitPlaque = { ...clean, originUrl: '' };
		expect(gitSyncLabel(noOrigin)).toBe('no origin');
		expect(gitNeedsAttention(noOrigin)).toBe(true);

		expect(gitSyncLabel(null)).toBe('—');
		expect(gitMeta(null, 'git failed')).toBe('git failed');
		expect(gitDetail(null, '')).toBe('');
	});
});

describe('dashboard projections', () => {
	it('stamps row timestamps in 24-hour local time', () => {
		expect(formatTimestamp('2026-09-25T18:04:22Z')).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
		expect(formatTimestamp('2026-09-25T18:04:22Z')).toMatch(/ (0[0-9]|1[0-9]|2[0-3]):\d{2}:\d{2}$/);
		expect(formatTimestamp(undefined)).toBe('—');
		expect(formatTimestamp('not-a-date')).toBe('—');
	});

	it('maps the NATS report to a transport status and an operator word', () => {
		expect(natsStatus(undefined)).toBe('unknown');
		expect(natsStatus({ configured: true, connected: true, ok: true })).toBe('connected');
		expect(natsStatus({ configured: true, connected: false, ok: false })).toBe('disconnected');
		expect(natsStatus({ configured: false, connected: false, ok: false })).toBe('idle');
	});

	it('keeps the internal idle status out of the NATS label', () => {
		expect(natsLabel('connected')).toBe('connected');
		expect(natsLabel('disconnected')).toBe('disconnected');
		expect(natsLabel('idle')).toBe('not configured');
		expect(natsLabel('unknown')).toBe('unknown');
		expect(natsLabel('reconnecting')).toBe('unknown');
	});

	it('sorts task counts so the tile reads the same on every poll', () => {
		expect(taskCountEntries({ running: 2, blocked: 5, done: 1 })).toEqual([
			['blocked', 5],
			['done', 1],
			['running', 2]
		]);
		expect(taskCountEntries({})).toEqual([]);
		expect(taskCountEntries(undefined)).toEqual([]);
	});

	it('reads a trace as active, failed, or idle', () => {
		const active = traceSummary({ hasActive: true, hasFailures: true, runCount: 6 });
		expect(traceState(active)).toBe('active');
		expect(traceStateLabel(active)).toBe('active');

		const failed = traceSummary({ hasActive: false, hasFailures: true, runCount: 6 });
		expect(traceState(failed)).toBe('failed');
		expect(traceStateLabel(failed)).toBe('failures');

		expect(traceState(traceSummary({ hasActive: false, hasFailures: false, runCount: 3 }))).toBe('idle');
		expect(traceStateLabel(traceSummary({ hasActive: false, hasFailures: false, runCount: 3 }))).toBe(
			'3 runs'
		);
		expect(traceStateLabel(traceSummary({ hasActive: false, hasFailures: false, runCount: 1 }))).toBe(
			'1 run'
		);
	});

	it('prefers the trace title and falls back to the id', () => {
		expect(tracePrimaryLabel(traceSummary())).toBe('Refactor the adapter seam');
		expect(tracePrimaryLabel(traceSummary({ title: '' }))).toBe('trace-01a0bd6963faa14f');
	});

	it('builds the trace meta line with counts that agree in number', () => {
		expect(traceMeta(traceSummary({ runCount: 4, taskCount: 2 }))).toMatch(
			/^4 runs · 2 tasks · \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
		);
		expect(traceMeta(traceSummary({ runCount: 1, taskCount: 1, lastActivityAt: '' }))).toBe('1 run · 1 task');
	});

	it('keeps only the runs that need attention on the dashboard', () => {
		expect(runStateLabel('failed')).toBe('failed');
		expect(runStateLabel('')).toBe('unknown');
		expect(runNeedsAttention(runSummary({ state: 'failed' }))).toBe(true);
		expect(runNeedsAttention(runSummary({ state: 'cancelled' }))).toBe(true);
		expect(runNeedsAttention(runSummary({ state: 'killed' }))).toBe(true);
		expect(runNeedsAttention(runSummary({ state: 'success' }))).toBe(false);

		const summary = dashboardSummary({
			failedRuns: [
				runSummary({ agentId: 'a', state: 'failed' }),
				runSummary({ agentId: 'b', state: 'cancelled' }),
				runSummary({ agentId: 'c', state: 'success' })
			]
		});
		expect(dashboardFailedRuns(summary).map((run) => run.agentId)).toEqual(['a', 'b']);
		expect(dashboardFailedRuns(null)).toEqual([]);
	});

	it('stamps an insight with a 24-hour local timestamp', () => {
		expect(insightTime(insightHighlight())).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
	});
});
