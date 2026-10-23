<script lang="ts">
	import Section from '$lib/components/Section.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import type { TraceStore } from '$lib/stores/trace.svelte';
	import { formatTimestamp } from '$lib/format';
	import type { MetaRow } from '$lib/format';
	import type { ConfigChange, TrailConfigApplyRequest } from '$lib/api/types';

	let { store }: { store: TraceStore } = $props();

	let source = $state<'colony' | 'snapshot' | 'overrides'>('colony');
	let snapshotRef = $state('');
	let overridesText = $state('');
	let reason = $state('');
	let localError = $state('');

	const detail = $derived(store.detail);
	const manifest = $derived(detail?.config ?? null);
	const policy = $derived(manifest?.policy ?? detail?.configPolicy ?? 'editable');
	const pinned = $derived(policy === 'pinned');
	const pending = $derived(manifest?.pending ?? detail?.pendingConfig ?? null);
	const history = $derived([...(manifest?.history ?? [])].reverse());
	const preview = $derived(store.configPreview);

	const configRows: MetaRow[] = $derived(
		manifest
			? [
					{
						label: 'Active',
						value: manifest.activeConfigRef || '—',
						mono: true,
						hint: manifest.activeConfigRef ? [manifest.activeConfigRef] : undefined,
						copy: Boolean(manifest.activeConfigRef)
					},
					{
						label: 'Initial',
						value: manifest.initialConfigRef || '—',
						mono: true,
						hint: manifest.initialConfigRef ? [manifest.initialConfigRef] : undefined,
						copy: Boolean(manifest.initialConfigRef)
					},
					{ label: 'Revision', value: String(manifest.configRevision) },
					{ label: 'Snapshots', value: String(manifest.snapshots?.length ?? 0) },
					...(manifest.workspaceRoot
						? [{ label: 'Workspace', value: manifest.workspaceRoot, mono: true, copy: true }]
						: [])
				]
			: []
	);

	function parseOverrides(text: string): { role: string; key: string; value: string }[] {
		const overrides: { role: string; key: string; value: string }[] = [];
		for (const raw of text.split('\n')) {
			const line = raw.trim();
			if (line === '') continue;
			const equals = line.indexOf('=');
			if (equals < 0) throw new Error(`Invalid override ${line}: expected bee.<role>.<param>=<value>`);
			const dotted = line.slice(0, equals).trim().split('.');
			if (dotted.length !== 3 || dotted[0] !== 'bee' || !dotted[1] || !dotted[2]) {
				throw new Error(`Invalid override ${line}: expected bee.<role>.<param>=<value>`);
			}
			overrides.push({
				role: dotted[1],
				key: dotted[2],
				value: line.slice(equals + 1).trim()
			});
		}
		return overrides;
	}

	async function submit(dryRun: boolean): Promise<void> {
		localError = '';
		const body: TrailConfigApplyRequest = {
			source,
			reason: reason.trim(),
			dryRun,
			expectedRevision: manifest?.configRevision
		};
		try {
			if (source === 'snapshot') {
				if (!snapshotRef) throw new Error('Choose a retained snapshot to activate.');
				body.snapshotRef = snapshotRef;
			}
			if (source === 'overrides') {
				body.overrides = parseOverrides(overridesText);
				if (body.overrides.length === 0) throw new Error('Add at least one bee.<role>.<param>=<value> override.');
			}
		} catch (cause) {
			localError = cause instanceof Error ? cause.message : String(cause);
			return;
		}
		await store.applyConfig(body);
	}

	async function cancelPending(): Promise<void> {
		if (!pending?.changeId) return;
		await store.cancelConfig(pending.changeId, reason.trim() || 'cancelled from Queen Console');
	}

	function changeMeta(change: ConfigChange): string {
		return [
			change.actor ? `by ${change.actor}` : '',
			change.source ? `source ${change.source}` : '',
			change.detail ? change.detail : ''
		]
			.filter(Boolean)
			.join(' · ');
	}
</script>

<Section id="trace-config" title="Configuration" note={detail?.configBound ? `revision ${manifest?.configRevision ?? detail?.configRevision}` : 'unbound'}>
	{#if !detail?.configBound || !manifest}
		<p class="text-sm text-base-content/60">
			This trail has no configuration binding. Legacy trails keep the colony configuration they
			started under; there is no immutable snapshot to preview or apply.
		</p>
	{:else}
		<div class="space-y-3">
			<div class="flex flex-wrap items-center gap-2">
				<StatusBadge
					status={pinned ? 'pinned' : 'editable'}
					label={pinned ? 'pinned experiment' : 'editable'}
				/>
				{#if pending}
					<StatusBadge status="pending" label="change pending" />
				{/if}
				<span class="text-xs text-base-content/50">Updated {formatTimestamp(manifest.updatedAt)}</span>
			</div>

			<MetaList rows={configRows} label="Configuration binding" />

			{#if pinned}
				<div class="alert alert-warning" role="status">
					<span>
						This trail is pinned to its initial configuration. Applying a newer colony
						configuration is refused; create or inspect forks for an experiment instead.
					</span>
				</div>
			{/if}

			{#if pending}
				<div class="rounded-box border border-warning/40 bg-warning/10 p-3">
					<div class="flex flex-wrap items-start justify-between gap-2">
						<div class="min-w-0">
							<p class="text-sm font-medium">
								Pending {pending.source || 'configuration'} change
								{pending.changeId ? `· ${pending.changeId}` : ''}
							</p>
							<p class="text-xs text-base-content/60">
								Expected revision {pending.expectedRevision ?? '—'} · waits for a safe boundary
							</p>
							{#if pending.blockers?.length}
								<ul class="mt-1 list-inside list-disc text-xs text-warning">
									{#each pending.blockers as blocker (blocker)}
										<li>{blocker}</li>
									{/each}
								</ul>
							{/if}
						</div>
						<button
							type="button"
							class="btn btn-sm btn-warning btn-outline"
							disabled={store.cancelPending}
							onclick={() => void cancelPending()}
						>
							{store.cancelPending ? 'Cancelling…' : 'Cancel pending change'}
						</button>
					</div>
				</div>
			{/if}

			{#if store.configError}
				<p class="text-sm text-error" role="alert">{store.configError}</p>
			{/if}
			{#if localError}
				<p class="text-sm text-error" role="alert">{localError}</p>
			{/if}

			{#if preview}
				<div class="rounded-box border border-base-300 p-3" data-config-preview>
					<div class="flex flex-wrap items-center gap-2">
						<StatusBadge status={preview.change.outcome ?? 'pending'} label={preview.dryRun ? 'preview' : (preview.change.outcome ?? 'result')} />
						{#if preview.change.detail}
							<span class="text-xs text-base-content/60">{preview.change.detail}</span>
						{/if}
						{#if preview.newConfigRef}
							<span class="font-mono text-xs text-base-content/50">{preview.newConfigRef}</span>
						{/if}
					</div>
					{#if preview.dryRun}
						<p class="mt-1 text-xs text-base-content/60">Preview only — nothing was written.</p>
					{/if}
					{#if preview.change.blockers?.length}
						<ul class="mt-2 list-inside list-disc text-xs text-warning">
							{#each preview.change.blockers as blocker (blocker)}
								<li>{blocker}</li>
							{/each}
						</ul>
					{/if}
					{#if preview.remainingWork.problems?.length}
						<ul class="mt-2 list-inside list-disc text-xs text-error">
							{#each preview.remainingWork.problems as problem (problem)}
								<li>{problem}</li>
							{/each}
						</ul>
					{/if}
					{#if preview.effectiveDiff?.length}
						<p class="mt-2 text-xs font-medium text-base-content/60">Effective change</p>
						<ul class="font-mono text-xs">
							{#each preview.effectiveDiff as line (line)}
								<li>{line}</li>
							{/each}
						</ul>
					{/if}
					{#if preview.shadowedDiff?.length}
						<p class="mt-2 text-xs font-medium text-base-content/60">Shadowed by a frozen layer</p>
						<ul class="font-mono text-xs text-base-content/60">
							{#each preview.shadowedDiff as line (line)}
								<li>{line}</li>
							{/each}
						</ul>
					{/if}
					{#if preview.diff?.length}
						<p class="mt-2 text-xs font-medium text-base-content/60">File inventory delta</p>
						<ul class="font-mono text-xs text-base-content/60">
							{#each preview.diff as line (line)}
								<li>{line}</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/if}

			<form
				class="space-y-3 rounded-box border border-base-300 p-3"
				onsubmit={(event) => {
					event.preventDefault();
					void submit(false);
				}}
			>
				<p class="text-sm font-medium">Apply a configuration version</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<label class="fieldset">
						<span class="fieldset-legend text-xs">Source</span>
						<select class="select select-sm" bind:value={source} disabled={pinned || Boolean(pending)}>
							<option value="colony">Current colony configuration</option>
							<option value="snapshot">Retained snapshot</option>
							<option value="overrides">Trail-local overrides</option>
						</select>
					</label>
					{#if source === 'snapshot'}
						<label class="fieldset">
							<span class="fieldset-legend text-xs">Snapshot</span>
							<select class="select select-sm" bind:value={snapshotRef} disabled={pinned || Boolean(pending)}>
								<option value="">Choose a retained snapshot</option>
								{#each manifest.snapshots ?? [] as snapshot (snapshot.configRef)}
									<option value={snapshot.configRef}>{snapshot.configRef}</option>
								{/each}
							</select>
						</label>
					{/if}
				</div>
				{#if source === 'overrides'}
					<label class="fieldset">
						<span class="fieldset-legend text-xs">Overrides, one per line</span>
						<textarea
							class="textarea textarea-sm font-mono"
							rows="3"
							placeholder="bee.builder.model=opencode/fast"
							bind:value={overridesText}
							disabled={pinned || Boolean(pending)}
						></textarea>
					</label>
				{/if}
				<label class="fieldset">
					<span class="fieldset-legend text-xs">Reason (recorded)</span>
					<input class="input input-sm" bind:value={reason} placeholder="why this version" disabled={pinned || Boolean(pending)} />
				</label>
				<div class="flex flex-wrap items-center gap-2">
					<button
						type="button"
						class="btn btn-sm"
						disabled={pinned || Boolean(pending) || store.configPending}
						onclick={() => void submit(true)}
					>
						Preview change
					</button>
					<button
						type="submit"
						class="btn btn-sm btn-primary"
						disabled={pinned || Boolean(pending) || store.configPending}
					>
						{store.configPending ? 'Working…' : 'Apply configuration'}
					</button>
					<span class="text-xs text-base-content/50">
						Trail revision {manifest.configRevision}; stale revisions are refused and recorded.
					</span>
				</div>
			</form>

			<Section id="trace-config-history" title="Change history" note={history.length > 0 ? String(history.length) : undefined} collapsible open={false}>
				{#if history.length === 0}
					<p class="text-sm text-base-content/60">No configuration changes recorded.</p>
				{:else}
					<ul class="space-y-2">
						{#each history as change (change.changeId ?? `${change.revision}-${change.createdAt}`)}
							<li class="rounded-box border border-base-300 px-3 py-2" data-config-change={change.changeId ?? ''}>
								<div class="flex flex-wrap items-start justify-between gap-2">
									<div class="min-w-0">
										<p class="text-sm">
											Revision {change.revision} · {change.source || 'configuration'}
											{#if change.changeId}<span class="font-mono text-xs text-base-content/50"> · {change.changeId}</span>{/if}
										</p>
										{#if changeMeta(change)}
											<p class="text-xs text-base-content/60">{changeMeta(change)}</p>
										{/if}
										{#if change.reason}
											<p class="text-xs text-base-content/60">“{change.reason}”</p>
										{/if}
										{#if change.newConfigRef}
											<p class="truncate font-mono text-xs text-base-content/50">{change.newConfigRef}</p>
										{/if}
										{#if change.blockers?.length}
											<ul class="mt-1 list-inside list-disc text-xs text-warning">
												{#each change.blockers as blocker (blocker)}
													<li>{blocker}</li>
												{/each}
											</ul>
										{/if}
									</div>
									<div class="flex shrink-0 flex-col items-end gap-1">
										<StatusBadge status={change.outcome ?? 'unknown'} label={change.outcome ?? 'unknown'} />
										<span class="text-xs text-base-content/50">{formatTimestamp(change.createdAt)}</span>
									</div>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</Section>
		</div>
	{/if}
</Section>
