import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import TrailConfigPanel from './TrailConfigPanel.svelte';
import TrailForkPanel from './TrailForkPanel.svelte';
import { createTraceStore, type TraceStore } from '$lib/stores/trace.svelte';
import {
	configManifest,
	forkLineage,
	forkRequest,
	trailConfigApplyResult,
	traceDetail
} from '../../../tests/fixtures';
import type {
	ConfigManifest,
	ForkDecision,
	ForkRequest,
	TraceDetail,
	TrailConfigApplyResult
} from '$lib/api/types';

const traceId = 'trace-01a0bd6963faa14f';

function traceStore(options: {
	detail: TraceDetail;
	applyConfig?: (traceId: string, body: unknown) => Promise<TrailConfigApplyResult>;
	cancelConfig?: (traceId: string, body: { changeId?: string; reason?: string }) => Promise<ConfigManifest>;
	requests?: ForkRequest[];
	decideFork?: (requestId: string, action: string, reason: string) => Promise<ForkDecision>;
}): TraceStore {
	return createTraceStore({
		loadTrace: async () => options.detail,
		loadArtifacts: async () => [],
		loadForks: async () => [],
		loadForkRequests: async () => options.requests ?? [],
		applyConfig: options.applyConfig,
		cancelConfig: options.cancelConfig,
		decideFork: options.decideFork,
		pollIntervalMs: 0
	});
}

async function loadedStore(options: Parameters<typeof traceStore>[0]): Promise<TraceStore> {
	const store = traceStore(options);
	await store.load(traceId);
	return store;
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('trail configuration panel', () => {
	it('previews a colony apply against the loaded revision, then applies it', async () => {
		const user = userEvent.setup();
		const applyConfig = vi.fn(async (_traceId: string, _body: unknown) =>
			trailConfigApplyResult({ dryRun: true })
		);
		const store = await loadedStore({
			detail: traceDetail({
				configBound: true,
				configRevision: 2,
				configPolicy: 'editable',
				config: configManifest()
			}),
			applyConfig
		});
		render(TrailConfigPanel, { store });

		await waitFor(() => expect(screen.getByText('revision 2')).toBeInTheDocument());
		await user.type(screen.getByPlaceholderText('why this version'), 'adopt it');
		await user.click(screen.getByRole('button', { name: 'Preview change' }));

		expect(applyConfig).toHaveBeenCalledWith(
			traceId,
			expect.objectContaining({ source: 'colony', expectedRevision: 2, dryRun: true, reason: 'adopt it' })
		);
		expect(await screen.findByText('Preview only — nothing was written.')).toBeInTheDocument();
		expect(screen.getByText('scout.model: old -> new')).toBeInTheDocument();

		applyConfig.mockResolvedValueOnce(trailConfigApplyResult({ dryRun: false }));
		await user.click(screen.getByRole('button', { name: 'Apply configuration' }));
		expect(applyConfig).toHaveBeenLastCalledWith(
			traceId,
			expect.objectContaining({ dryRun: false, expectedRevision: 2 })
		);
	});

	it('refuses to build a form that would apply on a pinned trail', async () => {
		const store = await loadedStore({
			detail: traceDetail({
				configBound: true,
				configRevision: 2,
				configPolicy: 'pinned',
				config: configManifest({ policy: 'pinned' })
			})
		});
		render(TrailConfigPanel, { store });

		expect(await screen.findByText(/This trail is pinned to its initial configuration/)).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Preview change' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Apply configuration' })).toBeDisabled();
		expect(screen.getByText('pinned experiment')).toBeInTheDocument();
	});

	it('cancels the one pending change and shows its recorded blockers', async () => {
		const user = userEvent.setup();
		const pending = configManifest();
		pending.pending = {
			revision: 2,
			initialConfigRef: pending.initialConfigRef,
			activeConfigRef: pending.activeConfigRef,
			changeId: 'change-pending',
			source: 'colony',
			expectedRevision: 2,
			outcome: 'pending',
			blockers: ['task task-1 is running'],
			createdAt: '2026-09-25T18:10:00Z'
		};
		const cancelConfig = vi.fn(async () => ({ ...pending, pending: undefined }));
		const store = await loadedStore({
			detail: traceDetail({
				configBound: true,
				configRevision: 2,
				configPolicy: 'editable',
				pendingConfig: pending.pending,
				config: pending
			}),
			cancelConfig
		});
		render(TrailConfigPanel, { store });

		expect(await screen.findByText('task task-1 is running')).toBeInTheDocument();
		await user.click(screen.getByRole('button', { name: 'Cancel pending change' }));

		expect(cancelConfig).toHaveBeenCalledWith(traceId, {
			changeId: 'change-pending',
			reason: 'cancelled from Queen Console'
		});
		await waitFor(() =>
			expect(screen.queryByRole('button', { name: 'Cancel pending change' })).not.toBeInTheDocument()
		);
	});
});

describe('trail fork panel', () => {
	it('links the source, shows the delta and limitations, and approves a pending request', async () => {
		const user = userEvent.setup();
		const request = forkRequest();
		const decideFork = vi.fn(async () => ({
			request: { ...request, state: 'approved' },
			lineage: forkLineage({ traceId: 'trace-fork-01a0bd6963faa150' }),
			created: true
		}));
		const lineage = forkLineage();
		const store = await loadedStore({
			detail: traceDetail({
				traceId: 'trace-fork-01a0bd6963faa150',
				forkOf: lineage.sourceTrace,
				forkId: lineage.forkId,
				configPolicy: 'pinned',
				admissionState: 'runnable',
				lineage,
				delta: lineage.delta
			}),
			requests: [request],
			decideFork
		});
		render(TrailForkPanel, { store });

		expect(await screen.findByRole('link', { name: /Source trail trace-01a0bd6963faa14f/ })).toHaveAttribute(
			'href',
			'/next/traces/trace-01a0bd6963faa14f'
		);
		expect(screen.getAllByText('patch').length).toBeGreaterThan(0);
		expect(screen.getByText('guard.model: old -> new')).toBeInTheDocument();
		expect(screen.getByText(/colony configuration is frozen at the source revision/)).toBeInTheDocument();

		await user.type(screen.getByPlaceholderText('Decision reason (recorded)'), 'looks safe');
		await user.click(screen.getByRole('button', { name: 'Approve and create fork' }));
		expect(decideFork).toHaveBeenCalledWith(request.requestId, 'approve', 'looks safe');
	});

	it('rejects a pending request with the recorded reason', async () => {
		const user = userEvent.setup();
		const request = forkRequest();
		const decideFork = vi.fn(async () => ({ request: { ...request, state: 'rejected' } }));
		const store = await loadedStore({
			detail: traceDetail(),
			requests: [request],
			decideFork
		});
		render(TrailForkPanel, { store });

		await user.type(screen.getByPlaceholderText('Decision reason (recorded)'), 'too broad');
		await user.click(screen.getByRole('button', { name: 'Reject' }));
		expect(decideFork).toHaveBeenCalledWith(request.requestId, 'reject', 'too broad');
	});
});
