<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { traceDetailPath } from '$lib/navigation';
	import { createForksStore, type ForksStore } from '$lib/stores/forks.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import { formatTimestamp, tracePrimaryLabel } from '$lib/format';
	import type { MetaRow } from '$lib/format';
	import type { ForkRequest, TraceSummary } from '$lib/api/types';

	let {
		store = createForksStore(),
		toasts = toastStore
	}: { store?: ForksStore; toasts?: ToastStore } = $props();

	let reason = $state('');

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const columns: DataColumn<TraceSummary>[] = [
		{
			key: 'fork',
			label: 'Experiment',
			text: (fork) => tracePrimaryLabel(fork),
			searchText: (fork) => [fork.traceId, fork.forkOf ?? '', fork.sourceTask ?? ''].join(' '),
			href: (fork) => traceDetailPath(base, fork.traceId),
			grow: true
		},
		{
			key: 'source',
			label: 'Source',
			text: (fork) => fork.forkOf ?? '',
			mono: true,
			href: (fork) => (fork.forkOf ? traceDetailPath(base, fork.forkOf) : null)
		},
		{
			key: 'delta',
			label: 'Delta',
			text: (fork) => fork.delta?.form ?? 'none',
			searchText: (fork) => fork.delta?.patch ?? ''
		},
		{
			key: 'state',
			label: 'Admission',
			text: (fork) => fork.configPolicy ?? fork.admissionState ?? 'runnable',
			badge: (fork) =>
				fork.configPolicy === 'pinned'
					? { status: 'pinned', label: 'pinned' }
					: { status: fork.admissionState || 'runnable', label: fork.admissionState || 'runnable' }
		},
		{
			key: 'budget',
			label: 'Budget',
			text: (fork) => (fork.budget ? String(fork.budget) : ''),
			secondary: true,
			align: 'right'
		},
		{
			key: 'activity',
			label: 'Last activity',
			text: (fork) => formatTimestamp(fork.lastActivityAt),
			secondary: true
		}
	];

	function requestTitle(req: ForkRequest): string {
		return req.evidence?.title || req.evidence?.task?.title || `Fork request ${req.requestId}`;
	}

	function requestRows(req: ForkRequest): MetaRow[] {
		const evidence = req.evidence;
		const rows: MetaRow[] = [
			{ label: 'Source', value: evidence?.sourceTrace || '—', mono: true },
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
		return rows;
	}

	async function decide(req: ForkRequest, action: 'approve' | 'reject' | 'cancel'): Promise<void> {
		const decision = await store.decide(req.requestId, action, reason.trim());
		if (!decision) {
			toasts.push('error', store.actionError || 'Fork request decision failed');
			return;
		}
		if (action === 'approve') {
			const target = decision.lineage?.traceId || decision.request.traceId || 'the new fork';
			toasts.push('success', `Approved ${req.requestId} — ${target}`);
		} else {
			toasts.push('success', `${action === 'reject' ? 'Rejected' : 'Cancelled'} ${req.requestId}`);
		}
	}
</script>

<svelte:head>
	<title>Forks · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-1">
		<h1 class="text-3xl font-bold">Forks</h1>
		<p class="text-base-content/70">
			Pending requests waiting for an operator, plus the experimental trails already running
			from a source.
		</p>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.loading}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-48 w-full"></span>
		</div>
	{:else}
		<Section
			id="fork-requests"
			title="Pending fork requests"
			note={store.requests.length > 0 ? String(store.requests.length) : undefined}
		>
			{#if store.actionError}
				<p class="mb-2 text-sm text-error" role="alert">{store.actionError}</p>
			{/if}
			{#if store.requests.length === 0}
				<p class="text-sm text-base-content/60">No fork request is waiting on an operator.</p>
			{:else}
				<ul class="space-y-3">
					{#each store.requests as req (req.requestId)}
						<li class="rounded-box border border-base-300 px-3 py-2" data-fork-request={req.requestId}>
							<div class="flex flex-wrap items-start justify-between gap-2">
								<div class="min-w-0">
									<p class="text-sm font-medium">{requestTitle(req)}</p>
									<p class="font-mono text-xs text-base-content/50">{req.requestId}</p>
								</div>
								<div class="flex flex-wrap items-center gap-2">
									{#if req.evidence?.configRef}
										<StatusBadge status="fork" label="configuration delta" />
									{/if}
									<StatusBadge status={req.state} label={req.state} />
								</div>
							</div>
							<div class="mt-2">
								<MetaList rows={requestRows(req)} label="Fork request evidence" />
							</div>
							<div class="mt-2 flex flex-wrap items-center gap-2">
								<input class="input input-sm w-full max-w-sm" bind:value={reason} placeholder="Decision reason (recorded)" />
								<button
									type="button"
									class="btn btn-sm btn-primary"
									disabled={store.decisionPending === req.requestId}
									onclick={() => void decide(req, 'approve')}
								>
									{store.decisionPending === req.requestId ? 'Working…' : 'Approve and create fork'}
								</button>
								<button
									type="button"
									class="btn btn-sm btn-error btn-outline"
									disabled={store.decisionPending === req.requestId}
									onclick={() => void decide(req, 'reject')}
								>
									Reject
								</button>
								<button
									type="button"
									class="btn btn-sm btn-ghost"
									disabled={store.decisionPending === req.requestId}
									onclick={() => void decide(req, 'cancel')}
								>
									Cancel request
								</button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</Section>

		<Section id="fork-list" title="Experimental trails" note={store.forks.length > 0 ? String(store.forks.length) : undefined}>
			<DataTable
				label="Experimental trails"
				{columns}
				rows={store.forks}
				rowKey={(fork) => fork.traceId}
				emptyMessage="No experimental forks yet."
				filterLabel="Filter forks"
				filterPlaceholder="title, source, delta"
				pageSize={15}
			/>
		</Section>
	{/if}
</div>
