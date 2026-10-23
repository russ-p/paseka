<script lang="ts">
	import { base } from '$app/paths';
	import DetailRow from '$lib/components/DetailRow.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { consolePath, traceDetailPath } from '$lib/navigation';
	import { formatTimestamp } from '$lib/format';
	import type { MetaRow } from '$lib/format';
	import type { TraceStore } from '$lib/stores/trace.svelte';
	import type { ForkRequest, TraceSummary } from '$lib/api/types';

	let { store }: { store: TraceStore } = $props();

	let reason = $state('');

	const detail = $derived(store.detail);
	const lineage = $derived(detail?.lineage ?? null);
	const isFork = $derived(Boolean(lineage || detail?.forkOf));
	const forks = $derived(store.forks);
	const requests = $derived(store.forkRequests);
	const visible = $derived(
		Boolean(lineage || detail?.forkOf || forks.length > 0 || requests.length > 0 || detail?.pendingForkRequests)
	);

	const lineageRows: MetaRow[] = $derived(
		lineage
			? [
					{ label: 'Fork', value: lineage.forkId, mono: true, copy: true },
					{ label: 'State', value: lineage.state || detail?.admissionState || 'runnable' },
					{ label: 'Source task', value: lineage.sourceTask || '—', mono: true, copy: Boolean(lineage.sourceTask) },
					{ label: 'Source attempt', value: lineage.sourceAttempt || '—', mono: true, copy: Boolean(lineage.sourceAttempt) },
					{ label: 'Source config', value: lineage.sourceConfigRef || '—', mono: true, hint: lineage.sourceConfigRef ? [lineage.sourceConfigRef] : undefined },
					{ label: 'Source revision', value: String(lineage.sourceRevision) },
					{ label: 'Candidate config', value: lineage.configRef || '—', mono: true, hint: lineage.configRef ? [lineage.configRef] : undefined },
					{ label: 'Base SHA', value: lineage.baseSha || '—', mono: true, copy: Boolean(lineage.baseSha) },
					{ label: 'Budget', value: String(lineage.budget ?? 0) },
					{ label: 'Branch', value: lineage.branch || '—', mono: Boolean(lineage.branch) },
					...(lineage.workspace ? [{ label: 'Workspace', value: lineage.workspace, mono: true, copy: true }] : []),
					{ label: 'Created', value: formatTimestamp(lineage.createdAt) }
				]
			: []
	);

	function requestTitle(req: ForkRequest): string {
		return req.evidence?.title || req.evidence?.task?.title || `Fork request ${req.requestId}`;
	}

	function requestRows(req: ForkRequest): MetaRow[] {
		const evidence = req.evidence;
		const rows: MetaRow[] = [
			{ label: 'Request', value: req.requestId, mono: true, copy: true },
			{ label: 'Source', value: evidence?.sourceTrace || '—', mono: true, copy: Boolean(evidence?.sourceTrace) },
			{ label: 'Source task', value: evidence?.sourceTask || '—', mono: true },
			{ label: 'Source attempt', value: evidence?.sourceAttempt || '—', mono: true },
			{ label: 'Delta', value: evidence?.delta?.form || 'none' },
			{ label: 'Budget', value: String(evidence?.budget ?? 0) },
			{ label: 'Validation', value: req.validation || 'recorded' },
			{ label: 'Expires', value: formatTimestamp(req.expiresAt) }
		];
		if (req.sender?.bee || req.sender?.agentId) {
			rows.push({
				label: 'Sender',
				value: [req.sender.bee, req.sender.agentId].filter(Boolean).join(' · '),
				mono: true
			});
		}
		if (evidence?.delta?.patch) rows.push({ label: 'Patch', value: evidence.delta.patch, mono: true });
		if (evidence?.delta?.rawDiff?.length) {
			rows.push({ label: 'Files', value: evidence.delta.rawDiff.join(', '), mono: true });
		}
		return rows;
	}

	async function decide(requestId: string, action: 'approve' | 'reject' | 'cancel'): Promise<void> {
		await store.decideFork(requestId, action, reason.trim());
	}

	function forkBadge(fork: TraceSummary): { status: string; label: string } {
		if (fork.configPolicy === 'pinned') return { status: 'pinned', label: 'pinned' };
		return { status: 'fork', label: 'fork' };
	}
</script>

{#if visible}
	<Section
		id="trace-fork"
		title="Fork and experiment"
		note={isFork ? 'experimental trail' : requests.length > 0 ? `${requests.length} pending` : undefined}
	>
		<div class="space-y-4">
			{#if isFork}
				<div class="space-y-3">
					<div class="flex flex-wrap items-center gap-2">
						<StatusBadge status="fork" label="experiment" />
						<StatusBadge status={detail?.configPolicy === 'pinned' ? 'pinned' : (detail?.admissionState || 'runnable')} label={detail?.configPolicy === 'pinned' ? 'pinned' : (detail?.admissionState || 'runnable')} />
						{#if detail?.forkOf}
							<a class="link text-sm" href={traceDetailPath(base, detail.forkOf)}>
								Source trail {detail.forkOf}
							</a>
						{/if}
					</div>
					{#if lineage}
						<MetaList rows={lineageRows} label="Fork lineage" />
						{#if lineage.delta}
							<div class="rounded-box border border-base-300 p-3">
								<p class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Delta</p>
								<p class="mt-1 text-sm">
									{lineage.delta.form || 'none'}
									{#if lineage.delta.proposalKind}<span class="text-base-content/60"> · {lineage.delta.proposalKind}</span>{/if}
									{#if lineage.delta.publication}<span class="text-base-content/60"> · {lineage.delta.publication}</span>{/if}
								</p>
								{#if lineage.delta.patch}
									<p class="truncate font-mono text-xs text-base-content/50">{lineage.delta.patch}</p>
								{/if}
								{#if lineage.delta.effectiveDiff?.length}
									<p class="mt-2 text-xs font-medium text-base-content/60">Effective configuration change</p>
									<ul class="font-mono text-xs">
										{#each lineage.delta.effectiveDiff as line (line)}
											<li>{line}</li>
										{/each}
									</ul>
								{/if}
								{#if lineage.delta.shadowedDiff?.length}
									<p class="mt-2 text-xs font-medium text-base-content/60">Shadowed by a frozen layer</p>
									<ul class="font-mono text-xs text-base-content/60">
										{#each lineage.delta.shadowedDiff as line (line)}
											<li>{line}</li>
										{/each}
									</ul>
								{/if}
								{#if lineage.delta.rawDiff?.length}
									<p class="mt-2 text-xs font-medium text-base-content/60">Files changed</p>
									<ul class="font-mono text-xs text-base-content/60">
										{#each lineage.delta.rawDiff as line (line)}
											<li>{line}</li>
										{/each}
									</ul>
								{/if}
							</div>
						{/if}
						{#if lineage.limitations?.length}
							<div class="alert alert-warning" role="status">
								<ul class="list-inside list-disc text-sm">
									{#each lineage.limitations as limitation (limitation)}
										<li>{limitation}</li>
									{/each}
								</ul>
							</div>
						{/if}
					{/if}
				</div>
			{/if}

			{#if forks.length > 0}
				<div>
					<p class="mb-2 text-sm font-medium">Forks of this trail</p>
					<ul class="space-y-2">
						{#each forks as fork (fork.traceId)}
							<DetailRow
								title={fork.title || fork.traceId}
								meta={[fork.traceId, fork.sourceTask ? `source ${fork.sourceTask}` : ''].filter(Boolean).join(' · ')}
								detail={fork.delta?.form ? `delta ${fork.delta.form}` : undefined}
								href={traceDetailPath(base, fork.traceId)}
								dataKey={fork.traceId}
							>
								{#snippet side()}
									<StatusBadge status={forkBadge(fork).status} label={forkBadge(fork).label} />
									{#if fork.pendingForkRequests}
										<StatusBadge status="pending" label={`${fork.pendingForkRequests} pending`} />
									{/if}
								{/snippet}
							</DetailRow>
						{/each}
					</ul>
				</div>
			{/if}

			{#if requests.length > 0}
				<div class="space-y-2" data-fork-requests>
					<div class="flex flex-wrap items-center justify-between gap-2">
						<p class="text-sm font-medium">Pending fork requests</p>
						<a class="link text-xs" href={consolePath(base, '/forks')}>Open the fork queue</a>
					</div>
					{#if store.forksError}
						<p class="text-sm text-error" role="alert">{store.forksError}</p>
					{/if}
					<ul class="space-y-2">
						{#each requests as req (req.requestId)}
							<li class="rounded-box border border-base-300 px-3 py-2" data-fork-request={req.requestId}>
								<div class="flex flex-wrap items-start justify-between gap-2">
									<div class="min-w-0">
										<p class="text-sm font-medium">{requestTitle(req)}</p>
										<p class="font-mono text-xs text-base-content/50">{req.requestId}</p>
									</div>
									<StatusBadge status={req.state} label={req.state} />
								</div>
								<div class="mt-2">
									<MetaList rows={requestRows(req)} label="Fork request evidence" columns={2} />
								</div>
								<div class="mt-2 flex flex-wrap items-center gap-2">
									<input class="input input-sm w-full max-w-sm" bind:value={reason} placeholder="Decision reason (recorded)" />
									<button
										type="button"
										class="btn btn-sm btn-primary"
										disabled={store.forkDecisionPending === req.requestId}
										onclick={() => void decide(req.requestId, 'approve')}
									>
										{store.forkDecisionPending === req.requestId ? 'Working…' : 'Approve and create fork'}
									</button>
									<button
										type="button"
										class="btn btn-sm btn-error btn-outline"
										disabled={store.forkDecisionPending === req.requestId}
										onclick={() => void decide(req.requestId, 'reject')}
									>
										Reject
									</button>
									<button
										type="button"
										class="btn btn-sm btn-ghost"
										disabled={store.forkDecisionPending === req.requestId}
										onclick={() => void decide(req.requestId, 'cancel')}
									>
										Cancel request
									</button>
								</div>
							</li>
						{/each}
					</ul>
				</div>
			{:else if forks.length === 0 && !isFork && detail?.pendingForkRequests}
				<p class="text-sm text-base-content/60">A fork request is pending but could not be read.</p>
			{/if}
		</div>
	</Section>
{/if}
