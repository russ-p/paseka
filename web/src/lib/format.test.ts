import { describe, expect, it } from 'vitest';
import type { AgentItem, GitPlaque, HostStatus, RuntimeStatus } from '$lib/api/types';
import {
	dashboardSummary,
	gitBranch,
	gitView,
	gitWorktree,
	insightHighlight,
	protocolEvent,
	runSummary,
	taskDetail,
	taskListItem,
	systemView,
	topology,
	traceSummary
} from '../tests/fixtures';
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
	taskCardShowsId,
	taskIdentityRows,
	taskReviewLabel,
	taskRowMeta,
	taskRunMeta,
	taskStatusLabel,
	taskTitle,
	formatBytes,
	formatClock,
	formatFetchAge,
	formatPercent,
	formatProcessBytes,
	formatUptime,
	systemAvailableWord,
	systemCpuPending,
	systemDiskWord,
	systemIdentityRows,
	systemLoadWord,
	systemMemoryWord,
	liveBeePids,
	eventKindLine,
	activeFilterCount,
	eventFilterSummary,
	topologyCounts,
	runDurationMs,
	runEventSummary,
	runIdentityRows,
	runUsageRows,
	payloadDigest,
	gitDetail,
	gitMeta,
	gitNeedsAttention,
	gitSyncLabel,
	gitActionLabel,
	gitLeftoverNames,
	gitActionMessage,
	gitBranchFlags,
	gitBranchState,
	gitCloneRows,
	gitOriginRows,
	gitSyncWord,
	gitWorktreeState,
	hostBadge,
	hostDetail,
	hostLoad,
	hostMeta,
	runtimeMeta,
	taskCountEntries,
	traceMeta,
	tracePrimaryLabel,
	traceState,
	traceStateLabel,
	traceBees,
	traceFlags,
	energyAvailable,
	energyDenominator,
	energyLabel,
	energyLowLabel,
	energyMetaLabel,
	formatDuration,
	formatTokenCount,
	artifactLabel,
	artifactMeta,
	artifactState,
	runMeta,
	runUsageLabel,
	taskMeta,
	taskPrimaryLabel,
	usageRows
} from './format';
import type { MetaRow } from './format';

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

describe('honey reserve formatting', () => {
	it('caps a standing trail by its stipend and any other by what was added', () => {
		expect(energyDenominator({ standing: true, energyBudget: 12, energyAllocated: 30 })).toBe(12);
		expect(energyDenominator({ energyBudget: 4, energyAdded: 6, energyAllocated: 10 })).toBe(10);
		expect(energyDenominator({ energyBudget: 4 })).toBe(4);
		expect(energyDenominator({})).toBe(0);
	});

	it('reads remaining over denominator, or bare when there is no budget', () => {
		expect(energyLabel({ energyBudget: 12, energyRemaining: 3 })).toBe('3 / 12');
		expect(energyLabel({ energyRemaining: 4 })).toBe('4');
	});

	it('names the provenance of the reserve', () => {
		expect(energyMetaLabel({ standing: true, energyBudget: 12 })).toBe('stipend 12');
		expect(energyMetaLabel({ energyBudget: 4, energyAdded: 6 })).toBe('seed 4 · topped 6');
		expect(energyMetaLabel({ energyBudget: 4 })).toBe('');
	});

	it('flags a low reserve and an absent one differently', () => {
		expect(energyLowLabel({ lowEnergy: true })).toBe('low');
		expect(energyLowLabel({})).toBeNull();
		expect(energyAvailable({ energyBudget: 12, energyRemaining: 3 })).toBe(true);
		expect(energyAvailable({ energyRemaining: 2 })).toBe(true);
		expect(energyAvailable({})).toBe(false);
	});
});

describe('usage formatting', () => {
	it('compacts token counts and keeps small ones exact', () => {
		expect(formatTokenCount(0)).toBe('0');
		expect(formatTokenCount(940)).toBe('940');
		expect(formatTokenCount(1234)).toBe('1.2k');
		expect(formatTokenCount(3_400_000)).toBe('3.4M');
		expect(formatTokenCount(undefined)).toBe('—');
	});

	it('scales a duration to the largest unit that reads', () => {
		expect(formatDuration(840)).toBe('840ms');
		expect(formatDuration(12_000)).toBe('12s');
		expect(formatDuration(185_000)).toBe('3m 5s');
		expect(formatDuration(7_800_000)).toBe('2h 10m');
		expect(formatDuration(undefined)).toBe('—');
	});

	it('turns an aggregate into MetaList rows and drops the runs count at zero', () => {
		expect(usageRows(undefined)).toEqual([]);
		expect(
			usageRows({
				inputTokens: 1200,
				outputTokens: 34,
				cacheReadTokens: 8000,
				cacheWriteTokens: 0,
				runCountWithUsage: 2
			})
		).toEqual([
			{ label: 'Input tokens', value: '1.2k' },
			{ label: 'Output tokens', value: '34' },
			{ label: 'Cache read', value: '8.0k' },
			{ label: 'Cache write', value: '0' },
			{ label: 'Runs with usage', value: '2' }
		]);
	});

	it('sums cache read and write into one run-row figure, and stays silent at zero', () => {
		expect(runUsageLabel({ inputTokens: 1200, outputTokens: 340, cacheReadTokens: 800 })).toBe(
			'1.2k in · 340 out · 800 cache'
		);
		expect(runUsageLabel({ inputTokens: 0, outputTokens: 0 })).toBe('');
		expect(runUsageLabel(undefined)).toBe('');
	});

	it('dates a run and adds its duration only when it finished', () => {
		const base = runSummary({
			startedAt: '2026-09-25T18:00:00Z',
			finishedAt: '2026-09-25T18:02:00Z',
			usage: { inputTokens: 100, outputTokens: 10 }
		});
		expect(runMeta(base)).toContain('100 in · 10 out');
		expect(runMeta(base)).toContain('2m 0s');
		expect(runMeta({ ...base, finishedAt: undefined })).not.toContain('2m');
	});
});

describe('trace row and comb formatting', () => {
	it('falls back to the identifier for an unnamed task', () => {
		expect(taskPrimaryLabel({ taskId: 'task-1', title: 'Ship it', status: 'ready' })).toBe('Ship it');
		expect(taskPrimaryLabel({ taskId: 'task-1', title: '', status: 'ready' })).toBe('task-1');
	});

	it('keeps the bee off a task that has none', () => {
		expect(taskMeta({ taskId: 'task-1', title: 'a', status: 'ready', bee: 'builder' })).toBe(
			'task-1 · builder'
		);
		expect(taskMeta({ taskId: 'task-1', title: 'a', status: 'ready' })).toBe('task-1');
	});

	it('prefers the author title for an artifact, then kind, then ref', () => {
		expect(artifactLabel({ ref: 'n.md', artifactKind: 'note', title: 'Notes' })).toBe('Notes');
		expect(artifactLabel({ ref: 'n.md', artifactKind: 'note' })).toBe('note');
		expect(artifactLabel({ ref: 'n.md', artifactKind: '' })).toBe('n.md');
	});

	it('joins kind, producer, and canonical comb path', () => {
		expect(artifactMeta({ ref: 'runs/t/artifacts/n.md', artifactKind: 'note', producer: 'a1' })).toBe(
			'note · a1 · runs/t/artifacts/n.md'
		);
		expect(artifactMeta({ ref: 'n.md', artifactKind: 'note' })).toBe('note · n.md');
	});

	it('separates an announced comb file from a staged one', () => {
		expect(artifactState({ announced: true })).toBe('announced');
		expect(artifactState({ announced: false })).toBe('staged');
	});

	it('lists the flags a trail actually carries', () => {
		expect(traceFlags(traceSummary({ standing: true, hasActive: true, hasFailures: true }))).toEqual([
			'standing',
			'active',
			'failures'
		]);
		expect(traceFlags(traceSummary({ standing: false, hasActive: false, hasFailures: false }))).toEqual([]);
	});

	it('says so when a trail has picked no worker yet', () => {
		expect(traceBees(traceSummary({ bees: ['builder', 'guard'] }))).toBe('builder, guard');
		expect(traceBees(traceSummary({ bees: [] }))).toBe('—');
		expect(traceBees(traceSummary({ bees: undefined }))).toBe('—');
	});
});

describe('git route formatting', () => {
	/** Row lookup by label, so a test asserts on the value rather than the array order. */
	function row(rows: MetaRow[], label: string): string | undefined {
		return rows.find((entry) => entry.label === label)?.value;
	}

	it('says the clone stands uncompared instead of borrowing the branch name', () => {
		const never = gitView({ ahead: undefined, behind: undefined, dirty: false });
		expect(gitSyncWord(gitView())).toBe('↑3');
		expect(gitSyncWord(never)).toBe('not compared');
		expect(gitSyncWord(gitView({ ahead: undefined, behind: undefined, dirty: true }))).toBe(
			'dirty · not compared'
		);
		expect(gitSyncWord(gitView({ originUrl: undefined }))).toBe('no origin');
		expect(gitSyncWord(null)).toBe('—');
	});

	it('splits the clone from the origin so a two-column block pairs like with like', () => {
		const git = gitView();
		const clone = gitCloneRows(git);
		const origin = gitOriginRows(git);

		expect(clone.map((entry) => entry.label)).toEqual(['Branch', 'HEAD', 'Default branch', 'Working tree']);
		expect(origin.map((entry) => entry.label)).toEqual(['Sync', 'Origin', 'Last fetch']);
		// The divergence numbers live inside the sync word, so they are not rows of their own.
		expect(origin.some((entry) => entry.label === 'Ahead' || entry.label === 'Behind')).toBe(false);
		expect(row(origin, 'Sync')).toBe('↑3');
	});

	it('leaves out a row that would only say nothing', () => {
		const bare = gitView({
			dirty: false,
			lastFetchAgeSeconds: undefined,
			originUrl: undefined,
			note: undefined
		});

		expect(row(gitCloneRows(bare), 'Working tree')).toBeUndefined();
		expect(row(gitOriginRows(bare), 'Origin')).toBeUndefined();
		expect(row(gitOriginRows(bare), 'Last fetch')).toBeUndefined();
		expect(gitOriginRows(bare)).toEqual([{ label: 'Sync', value: 'no origin' }]);
		expect(gitCloneRows(null)).toEqual([]);
		expect(gitOriginRows(null)).toEqual([]);
	});

	it('keeps the server note, which is the only thing explaining an empty publish list', () => {
		const unfetched = gitView({
			ahead: undefined,
			behind: undefined,
			unpublished: undefined,
			note: 'fetch to update remote-tracking refs'
		});

		expect(row(gitOriginRows(unfetched), 'Note')).toBe('fetch to update remote-tracking refs');
	});

	it('makes the two values an operator pastes into a shell copyable and readable in full', () => {
		const clone = gitCloneRows(gitView());
		const origin = gitOriginRows(gitView());

		expect(row(clone, 'Branch')).toBe('main');
		expect(clone[0]).toMatchObject({ mono: true, copy: true, hint: ['main'] });
		expect(clone[1].hint).toEqual(['02453e88445ce591e01200c507bce35bc5071656']);
		expect(clone[1].value).toBe('02453e8');
		expect(row(origin, 'Origin')).toBe('git@github.com:russ-p/paseka.git');
		expect(origin[1]).toMatchObject({ mono: true, copy: true });
		// A short sha needs no hint, or the popover would repeat the visible line.
		expect(gitCloneRows(gitView({ headSha: '02453e8' }))[1].hint).toBeUndefined();
	});

	it('names one branch state, in the order the operator acts on it', () => {
		expect(gitBranchState(gitBranch())).toEqual({ status: 'active', label: 'current' });
		// Current wins over merged: you are standing on it, so that is the fact that matters.
		expect(gitBranchState(gitBranch({ merged: true, leftover: true }))).toEqual({
			status: 'active',
			label: 'current'
		});
		expect(gitBranchState(gitBranch({ current: false, leftover: true }))).toEqual({
			status: 'leftover',
			label: 'leftover'
		});
		expect(gitBranchState(gitBranch({ current: false, merged: true }))).toEqual({
			status: 'merged',
			label: 'merged'
		});
		expect(gitBranchState(gitBranch({ current: false }))).toBeNull();
	});

	it('keeps the two branch facts that are not state out of the badge', () => {
		expect(gitBranchFlags(gitBranch())).toBe('default');
		expect(gitBranchFlags(gitBranch({ default: false, worktreePath: '/colony/wt' }))).toBe('worktree');
		expect(
			gitBranchFlags(gitBranch({ default: false, worktreePath: '/colony/wt', current: false }))
		).toBe('worktree');
		expect(gitBranchFlags(gitBranch({ default: false }))).toBe('');
	});

	it('badges a worktree dirty or clean, so neither reads as an empty cell', () => {
		expect(gitWorktreeState(gitWorktree())).toEqual({ status: 'clean', label: 'clean' });
		expect(gitWorktreeState(gitWorktree({ dirty: true }))).toEqual({ status: 'dirty', label: 'dirty' });
	});

	it('names the branches the maintenance sweep is allowed to delete', () => {
		expect(
			gitLeftoverNames([
				gitBranch(),
				gitBranch({ name: 'paseka/a', merged: true, leftover: true }),
				gitBranch({ name: 'paseka/b', merged: true, leftover: true })
			])
		).toEqual(['paseka/a', 'paseka/b']);
		expect(gitLeftoverNames(undefined)).toEqual([]);
	});

	it('reports what a git POST did, preferring git output over the caller word', () => {
		expect(gitActionMessage({ ok: true, message: 'From github.com' }, 'Fetch complete')).toBe(
			'From github.com'
		);
		// `git worktree prune` and `git fetch` print nothing on success, so the fallback carries it.
		expect(gitActionMessage({ ok: true }, 'No orphan worktrees')).toBe('No orphan worktrees');
		expect(gitActionMessage(null, 'Pull complete')).toBe('Pull complete');
	});

	it('names the branches a batch delete lost instead of claiming it worked', () => {
		const partial = gitActionMessage(
			{
				ok: false,
				results: [
					{ name: 'paseka/a', ok: true },
					{ name: 'main', ok: false, error: 'branch is checked out' }
				]
			},
			'Merged leftovers deleted'
		);

		expect(partial).toBe('main: branch is checked out');
		expect(gitActionMessage({ ok: true, results: [{ name: 'paseka/a', ok: true }] }, 'x')).toBe('1 deleted');
		expect(
			gitActionMessage({ ok: false, results: [{ name: 'paseka/a', ok: false }] }, 'x')
		).toBe('paseka/a: failed');
	});

	it('names a button action in the present tense only while it is in flight', () => {
		expect(gitActionLabel('fetch', false)).toBe('Fetch');
		expect(gitActionLabel('fetch', true)).toBe('Fetching…');
		expect(gitActionLabel('prune', false)).toBe('Prune orphans');
		expect(gitActionLabel('prune', true)).toBe('Pruning…');
		expect(gitActionLabel('delete', true)).toBe('Deleting…');
	});
});

describe('formatProcessBytes', () => {
	it('picks the unit from the value, because most processes are MiB', () => {
		// The GiB formatter the metric tiles use would print 0.00 GiB down the
		// whole process column and make the rows incomparable. For the same reason
		// this never climbs back to GiB: a column that changes unit halfway down
		// is the thing it exists to prevent.
		expect(formatProcessBytes(1_073_741_824)).toBe('1024 MiB');
		expect(formatProcessBytes(214_958_080)).toBe('205 MiB');
		expect(formatProcessBytes(52_428_800)).toBe('50 MiB');
		expect(formatProcessBytes(5_242_880)).toBe('5.0 MiB');
		expect(formatProcessBytes(3_145_728)).toBe('3.0 MiB');
		expect(formatProcessBytes(65_536)).toBe('64 KiB');
		expect(formatProcessBytes(512)).toBe('512 B');
	});

	it('is null when the server could not measure the process', () => {
		expect(formatProcessBytes(undefined)).toBeNull();
		expect(formatProcessBytes(Number.NaN)).toBeNull();
	});
});

describe('system metric words', () => {
	const host: HostStatus = {
		os: 'linux',
		arch: 'amd64',
		cpus: 8,
		load1: 1.424,
		load5: 0.981,
		load15: 0.612,
		cpuPercent: 18,
		memUsedBytes: 6_442_450_944,
		memTotalBytes: 17_179_869_184,
		memAvailableBytes: 10_737_418_240,
		diskUsedBytes: 42_949_672_960,
		diskTotalBytes: 214_748_364_800
	};

	it('pairs used with total and leaves a half-measured figure unsaid', () => {
		expect(systemMemoryWord(host)).toBe('6.00 GiB / 16.0 GiB');
		expect(systemDiskWord(host)).toBe('40.0 GiB / 200 GiB');
		expect(systemAvailableWord(host)).toBe('10.0 GiB');
		expect(systemMemoryWord({ ...host, memTotalBytes: undefined })).toBeNull();
		expect(systemDiskWord({ ...host, diskUsedBytes: undefined })).toBeNull();
		expect(systemMemoryWord(null)).toBeNull();
	});

	it('reports the three averages together, because a partial set is not a load', () => {
		expect(systemLoadWord(host)).toBe('1.42 / 0.98 / 0.61');
		expect(systemLoadWord({ ...host, load15: undefined })).toBeNull();
		expect(systemLoadWord(null)).toBeNull();
	});

	it('treats a missing cpu percent as the first sample, not a fault', () => {
		// CPU percent is a delta between two /proc/stat reads, so the first poll of a
		// fresh console cannot have one.
		expect(systemCpuPending({ ...host, cpuPercent: undefined })).toBe(true);
		expect(systemCpuPending(host)).toBe(false);
		expect(systemCpuPending(null)).toBe(false);
	});
});

describe('systemIdentityRows', () => {
	it('names the box and offers the values an operator pastes elsewhere', () => {
		const rows = systemIdentityRows(systemView());

		expect(rows.map((row) => row.label)).toEqual([
			'Hostname',
			'Kernel',
			'OS / arch',
			'CPUs',
			'Uptime',
			'Console PID',
			'Go'
		]);
		const hostname = rows[0];
		expect(hostname.value).toBe('apiary');
		expect(hostname.copy).toBe(true);
		expect(hostname.mono).toBe(true);
		expect(rows.find((row) => row.label === 'OS / arch')?.value).toBe('linux / amd64');
		expect(rows.find((row) => row.label === 'CPUs')?.value).toBe('8');
		expect(rows.find((row) => row.label === 'Uptime')?.value).toBe('7d 0h');
		expect(rows.find((row) => row.label === 'Console PID')?.mono).toBe(true);
	});

	it('drops the rows a degraded snapshot could not measure', () => {
		// Off Linux the identity fields still work while load, memory, and the
		// process list do not, so the block must not be padded with dashes.
		const rows = systemIdentityRows({
			os: 'darwin',
			arch: 'arm64',
			cpus: 10,
			consolePid: 4242
		});

		expect(rows.map((row) => row.label)).toEqual(['OS / arch', 'CPUs', 'Console PID']);
	});

	it('is empty with no snapshot at all', () => {
		expect(systemIdentityRows(null)).toEqual([]);
	});
});

describe('liveBeePids', () => {
	const item = (pid: number): AgentItem => ({
		kind: 'afk',
		bee: 'scout',
		pid,
		traceId: 'trace-01a0bd6963faa14f',
		agentId: 'agent-1',
		startedAt: '2026-09-25T18:04:22Z',
		runDir: '/colony/.paseka/runs/trace-01a0bd6963faa14f/agent-1'
	});

	it('marks the adapter pids the Live bees panel reports', () => {
		expect([...liveBeePids([item(4242), item(1187)])]).toEqual([4242, 1187]);
	});

	it('is an empty set when no bee is live, which is not an error', () => {
		expect(liveBeePids(undefined).size).toBe(0);
		expect(liveBeePids([]).size).toBe(0);
	});
});

describe('eventKindLine', () => {
	it('names the contract and the payload kind, which are different things', () => {
		expect(eventKindLine({ type: 'VERIFICATION', payloadKind: 'review.gate' })).toBe(
			'VERIFICATION · review.gate'
		);
	});

	it('leaves the contract alone rather than adding a dangling separator', () => {
		expect(eventKindLine({ type: 'SIGNAL' })).toBe('SIGNAL');
		expect(eventKindLine({ payloadKind: 'seam.note' })).toBe('seam.note');
		expect(eventKindLine({})).toBe('');
	});

	it('says the word once when the contract and the payload kind agree', () => {
		// The dashboard's insight projection carries one word for both, and
		// "INSIGHT · INSIGHT" is the kind of thing a reviewer notices.
		expect(eventKindLine({ type: 'INSIGHT', payloadKind: 'INSIGHT' })).toBe('INSIGHT');
	});
});

describe('activeFilterCount', () => {
	it('counts what narrows the feed and ignores blank fields', () => {
		expect(activeFilterCount({})).toBe(0);
		expect(activeFilterCount({ traceId: 'trace-1' })).toBe(1);
		// A stray space is not a filter, and must not read as one in the summary line.
		expect(activeFilterCount({ traceId: 'trace-1', bee: '  ', type: '' })).toBe(1);
	});
});

describe('eventFilterSummary', () => {
	it('reads as a phrase naming what is narrowing the feed', () => {
		expect(
			eventFilterSummary({ traceId: 'trace-01a0bd6963faa14f', type: 'VERIFICATION', severity: 'high' })
		).toBe('trace-01a0bd6963faa14f · VERIFICATION · severity high');
	});

	it('labels the fields that are not self-describing', () => {
		expect(eventFilterSummary({ taskId: 'task-b2', bee: 'scout', kind: 'seam.note' })).toBe(
			'task task-b2 · scout · kind seam.note'
		);
	});

	it('is empty for the colony-wide feed, so the panel has nothing to say', () => {
		expect(eventFilterSummary({})).toBe('');
	});
});

describe('topologyCounts', () => {
	it('leads with bees, event kinds, and rules', () => {
		expect(topologyCounts(topology())).toEqual([
			{ label: 'Bees', value: '3' },
			{ label: 'Event kinds', value: '5' },
			{ label: 'Rules', value: '5' }
		]);
	});

	it('says nothing before there is a projection', () => {
		expect(topologyCounts(null)).toEqual([]);
	});
});

describe('runEventSummary', () => {
	it('projects a recorded event into a readable feed row', () => {
		expect(
			runEventSummary(
				protocolEvent({
					seq: 4,
					type: 'VERIFICATION',
					createdAt: '2026-09-25T18:04:00Z',
					payload: { kind: 'verification.success', summary: 'All requirements met.', severity: 'high' }
				})
			)
		).toEqual({
			createdAt: '2026-09-25T18:04:00Z',
			traceId: 'trace-01a0bd6963faa14f',
			agentId: 'run-01',
			type: 'VERIFICATION',
			payloadKind: 'verification.success',
			severity: 'high',
			summary: 'All requirements met.'
		});
	});

	it('falls back to the payload fields when an event carries no summary', () => {
		// `trace.title` announces a title and nothing else; a blank row would say
		// less than the payload already does.
		expect(
			runEventSummary(protocolEvent({ payload: { kind: 'trace.title', title: 'Add prune cleanup' } }))
				.summary
		).toBe('title=Add prune cleanup');
	});

	it('renders an event with neither a summary nor scalars as an empty row, not a crash', () => {
		const row = runEventSummary(protocolEvent({ payload: { kind: 'task.ready' } }));

		expect(row.summary).toBe('');
		expect(row.payloadKind).toBe('task.ready');
	});

	it('copes with a payload that is not an object at all', () => {
		expect(runEventSummary(protocolEvent({ payload: 'raw string' })).summary).toBe('');
		expect(runEventSummary(protocolEvent({ payload: undefined })).summary).toBe('');
	});
});

describe('payloadDigest', () => {
	it('lists the payload scalars in a stable order and counts the rest', () => {
		expect(payloadDigest({ kind: 'x', decision: 'grill', confidence: 0.9, extra: 'e' }, 2)).toBe(
			'confidence=0.9 · decision=grill · +1'
		);
	});

	it('reads a payload whose only content is a list of objects', () => {
		// `artifact.written` announces a list and has no top-level scalar, so a
		// scalars-only digest leaves the row blank.
		expect(
			payloadDigest({
				artifacts: [
					{
						artifactKind: 'review-report',
						ref: '.paseka/runs/trace-1/artifacts/review-report.md',
						title: 'Guard Review'
					}
				]
			})
		).toBe('artifacts:review-report · .paseka/runs/trace-1/artifacts/review-report.md · Guard Review');
	});

	it('skips a list of scalars rather than pretending it is a record', () => {
		expect(payloadDigest({ tags: ['a', 'b'] })).toBe('');
	});

	it('is empty for a payload with nothing but a kind', () => {
		expect(payloadDigest({ kind: 'task.ready' })).toBe('');
		expect(payloadDigest(undefined)).toBe('');
	});
});

describe('runDurationMs', () => {
	it('measures a finished run', () => {
		expect(
			runDurationMs({ startedAt: '2026-09-25T18:00:00Z', finishedAt: '2026-09-25T18:02:30Z' })
		).toBe(150_000);
	});

	it('is null while a run is still going, rather than reporting zero', () => {
		// A running run has no finish line, and a duration of zero reads as "instant".
		expect(runDurationMs({ startedAt: '2026-09-25T18:00:00Z', finishedAt: undefined })).toBeNull();
	});

	it('is null for a finish line that precedes the start', () => {
		expect(
			runDurationMs({ startedAt: '2026-09-25T18:02:00Z', finishedAt: '2026-09-25T18:00:00Z' })
		).toBeNull();
	});
});

describe('runIdentityRows', () => {
	it('names the run and links its trail through the caller href builder', () => {
		const rows = runIdentityRows(runSummary(), (id) => `/next/traces/${id}`);

		expect(rows.find((row) => row.label === 'Trail')).toMatchObject({
			value: 'trace-01a0bd6963faa14f',
			href: '/next/traces/trace-01a0bd6963faa14f'
		});
		// The agent id and the run directory are what an operator pastes elsewhere.
		expect(rows.find((row) => row.label === 'Agent')?.copy).toBe(true);
		expect(rows.find((row) => row.label === 'Run dir')?.copy).toBe(true);
	});

	it('leaves the trail unlinked when no builder is given', () => {
		const rows = runIdentityRows(runSummary());

		expect(rows.find((row) => row.label === 'Trail')?.href).toBeUndefined();
	});

	it('drops the rows a run never reported instead of padding with dashes', () => {
		const rows = runIdentityRows(
			runSummary({ intent: undefined, taskId: undefined, providerSessionId: undefined })
		);
		const labels = rows.map((row) => row.label);

		expect(labels).not.toContain('Intent');
		expect(labels).not.toContain('Task');
		expect(labels).not.toContain('Provider session');
		expect(labels).toContain('Duration');
	});

	it('says a run is still running rather than reporting no duration', () => {
		const rows = runIdentityRows(
			runSummary({ state: 'running', finishedAt: undefined })
		);

		expect(rows.find((row) => row.label === 'Duration')?.value).toBe('running');
		expect(rows.find((row) => row.label === 'Finished')).toBeUndefined();
	});

	it('is empty with no run', () => {
		expect(runIdentityRows(null)).toEqual([]);
	});
});

describe('runUsageRows', () => {
	it('lists the tokens a run reported', () => {
		const rows = runUsageRows({ inputTokens: 12_000, outputTokens: 900, cacheReadTokens: 40_000 });

		expect(rows.map((row) => row.label)).toEqual(['Input tokens', 'Output tokens', 'Cache read']);
	});

	it('leaves out a cache half the adapter did not report', () => {
		const rows = runUsageRows({ inputTokens: 10, outputTokens: 2, cacheReadTokens: 0 });

		expect(rows.map((row) => row.label)).toEqual(['Input tokens', 'Output tokens']);
	});

	it('is empty for a run that reported no usage', () => {
		expect(runUsageRows(undefined)).toEqual([]);
	});
});

describe('task formatters', () => {
	it('says the one status an operator cannot read as a single word in words', () => {
		// `waiting_review` is the status a board exists to make visible, so the badge
		// spells it; every other status is already one word and is passed through.
		expect(taskStatusLabel('waiting_review')).toBe('waiting review');
		expect(taskStatusLabel('ready')).toBe('ready');
		expect(taskStatusLabel('')).toBe('unknown');
	});

	it('says what a review policy means rather than printing the enum', () => {
		expect(taskReviewLabel('none')).toBe('not gated');
		expect(taskReviewLabel(undefined)).toBe('not gated');
		expect(taskReviewLabel('required')).toBe('required');
		// The final gate is the one that holds the whole trail, so it says so.
		expect(taskReviewLabel('final')).toBe('final gate');
	});

	it('names a task by its title, falling back to the id when there is none', () => {
		expect(taskTitle(taskListItem())).toBe('Wire the export format flag');
		expect(taskTitle(taskListItem({ title: '' }))).toBe('task-01');
	});

	it('puts the second line of a card in the order an operator reads it', () => {
		expect(taskRowMeta(taskListItem())).toBe('builder · api · 0 runs');
		expect(taskRowMeta(taskListItem({ sector: undefined, runCount: 1 }))).toBe('builder · 1 run');
		expect(taskRowMeta(taskListItem({ bee: undefined, sector: undefined, runCount: 2 }))).toBe(
			'2 runs'
		);
	});

	it('leaves dependencies out of the card\'s second line, because the badge has them', () => {
		// The card prints `after task-01, task-02` as its own badge already, and a
		// dependency printed twice on one card is noise rather than emphasis.
		expect(taskRowMeta(taskListItem({ dependsOn: ['task-01', 'task-02'], runCount: 3 }))).toBe(
			'builder · api · 3 runs'
		);
	});

	it('does not repeat a card\'s id line when the title already is the id', () => {
		expect(taskCardShowsId(taskListItem())).toBe(true);
		// A task the ledger named after its own trail arrives with title === taskId,
		// and printing it twice says nothing twice.
		expect(
			taskCardShowsId(taskListItem({ title: 'trace-01', taskId: 'trace-01' }))
		).toBe(false);
		// An empty title is printed as the id, so the id line would repeat that too.
		expect(taskCardShowsId(taskListItem({ title: '', taskId: 'task-01' }))).toBe(false);
	});

	it('leaves out identity rows a task does not carry', () => {
		const rows = taskIdentityRows(
			taskDetail({ sector: undefined, intent: undefined, commit: undefined })
		);
		const labels = rows.map((row) => row.label);

		// A row with nothing in it is noise, and every one of these is optional on the
		// wire: a task planned by a planner may have no sector, and one that has not
		// finished has no commit.
		expect(labels).not.toContain('Sector');
		expect(labels).not.toContain('Intent');
		expect(labels).not.toContain('Commit');
		expect(labels).toContain('Trail');
		expect(labels).toContain('Task');
	});

	it('offers the trail id a way out through the caller\'s builder', () => {
		const rows = taskIdentityRows(taskDetail(), (id) => `/next/traces/${id}`);

		expect(rows[0]).toEqual({
			label: 'Trail',
			value: 'trace-01a0bd6963faa14f',
			mono: true,
			href: '/next/traces/trace-01a0bd6963faa14f'
		});
		// The base path belongs to the caller, so a formatter never builds a URL.
		expect(taskIdentityRows(taskDetail())[0].href).toBeUndefined();
	});

	it('spells out the workspace, because isolated and root mean opposite things', () => {
		expect(
			taskIdentityRows(taskDetail({ proposalWorkspace: 'root' })).find(
				(row) => row.label === 'Workspace'
			)
		).toMatchObject({ value: 'root checkout' });
		expect(
			taskIdentityRows(taskDetail()).find((row) => row.label === 'Workspace')
		).toMatchObject({ value: 'isolated' });
	});

	it('says which ledger answered, because disk and JetStream are different facts', () => {
		expect(
			taskIdentityRows(taskDetail({ source: 'jetstream-kv' })).find(
				(row) => row.label === 'Ledger'
			)
		).toMatchObject({ value: 'jetstream kv' });
		expect(
			taskIdentityRows(taskDetail({ source: 'filesystem' })).find(
				(row) => row.label === 'Ledger'
			)
		).toMatchObject({ value: 'filesystem' });
	});

	it('returns nothing for a task that is not loaded', () => {
		expect(taskIdentityRows(null)).toEqual([]);
	});

	it('puts a linked run\'s time and directory on one line', () => {
		const run = taskDetail().runs[0];
		expect(taskRunMeta(run)).toContain('.paseka/runs/trace-01a0bd6963faa14f/run-02');
		// A run with neither a start time nor a directory leaves no empty separator.
		expect(taskRunMeta({ agentId: 'run-01' })).toBe('');
	});
});
