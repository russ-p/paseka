<script lang="ts">
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import DetailRow from '$lib/components/DetailRow.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import ThemeSelect from '$lib/components/ThemeSelect.svelte';
	import {
		configApiKeyLabel,
		configIsOverridden,
		configNatsHeadline,
		configNotifyEnabled,
		configProfileLabel,
		configSourceHint,
		configTelegramHeadline,
		configValueText
	} from '$lib/format';
	import { createSettingsStore, type SettingsStore } from '$lib/stores/settings.svelte';
	import type { ConfigAdapter } from '$lib/api/types';

	let { store = createSettingsStore() }: { store?: SettingsStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const config = $derived(store.config);

	const adapterColumns: DataColumn<ConfigAdapter>[] = [
		{
			key: 'name',
			label: 'Adapter',
			text: (adapter) => adapter.name,
			mono: true
		},
		{
			key: 'binary',
			label: 'Binary',
			// A default binary the operator never wrote says so in the cell, and the
			// filter box gets `default` for free because `text` feeds it.
			text: (adapter) => configValueText(adapter.binary),
			mono: true,
			grow: true
		},
		{
			key: 'apiKeyEnv',
			label: 'API key env',
			// The variable name, never the key: the credential model stores a
			// reference and the adapter reads the environment at call time, so a
			// key in this cell would be something the server deliberately never sent.
			text: (adapter) => adapter.apiKeyEnv.value || '—',
			mono: true
		},
		{
			key: 'key',
			label: 'Key',
			text: (adapter) => configApiKeyLabel(adapter),
			// The one fact that moves without a file changing, and it moves on its
			// own — a declared name is not a key, so only a resolving variable is news.
			badge: (adapter) =>
				adapter.apiKeyEnv.value === ''
					? null
					: adapter.apiKeySet
						? { status: 'connected', label: 'set' }
						: { status: 'unavailable', label: 'not set' }
		},
		{
			key: 'configured',
			label: 'File',
			// Whether the operator has a file to edit at all, which is a different
			// question from what the loader inferred — and the reason `Binary` can
			// carry a value nobody wrote.
			text: (adapter) => (adapter.configured ? 'present' : 'inferred')
		}
	];
</script>

<svelte:head>
	<title>Settings · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 max-w-3xl space-y-1">
			<h1 class="text-3xl font-bold">Settings</h1>
			<p class="text-base-content/70">{configNatsHeadline(config?.nats)}</p>
			<!-- Every value below is read-only in this preview, and the page says so
			     rather than implying an edit is one click away: the settings-write
			     endpoints have not landed, so an enabled-looking control here would
			     be a promise the server cannot keep. -->
			<p class="text-xs text-base-content/50">
				This view is read-only. Theme selection below is the one setting the console keeps
				itself; the rest is read from the colony and home config and changes when you edit
				those files or restart the console.
			</p>
		</div>
		<button
			id="settings-refresh"
			type="button"
			class="btn btn-sm"
			disabled={store.loading}
			onclick={() => void store.refresh()}
		>
			Refresh
		</button>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-24 w-full"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if config}
		<div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
			<Section id="settings-transport" title="Transport">
				<MetaList
					label="NATS transport"
					columns={1}
					rows={[
						{
							label: 'URL',
							value: configValueText(config.nats.url),
							mono: true,
							hint: configSourceHint(config.nats.url)
						},
						{
							label: 'Subject prefix',
							value: configValueText(config.nats.subjectPrefix),
							mono: true,
							hint: configSourceHint(config.nats.subjectPrefix)
						}
					]}
				/>
				{#if configIsOverridden(config.nats.url)}
					<!-- The whole point of carrying a source: an operator who edits the file
					     while an env var is set would otherwise see the file accepted and the
					     process unchanged. -->
					<div class="alert alert-warning mt-3" role="status">
						<span>
							The URL comes from <span class="font-mono">{config.nats.url.source.slice(4)}</span>,
							which outranks the home config. Editing <span class="font-mono">config.yaml</span>
							will not change it.
						</span>
					</div>
				{/if}
			</Section>

			<Section id="settings-theme" title="Appearance">
				<ThemeSelect />
			</Section>

			<Section
				id="settings-adapters"
				title="Adapters"
				class="xl:col-span-2"
				note="machine-local"
			>
				<!-- A credential is a reference here and in the adapter itself: what the
				     console shows is the variable the adapter reads, and whether it resolves. -->
				<p class="mb-3 text-xs text-base-content/50">
					Each adapter's key is read from a named environment variable at call time, so
					secrets are never stored in the colony config. The <b>File</b> column says whether
					the value came from <span class="font-mono">adapters/&lt;name&gt;.yaml</span> or was
					inferred from a default.
				</p>
				<DataTable
					label="Adapter credentials"
					columns={adapterColumns}
					rows={store.adapters}
					rowKey={(adapter) => adapter.name}
					emptyMessage="No adapters are configured for this colony."
					filterLabel="Filter adapters"
					filterPlaceholder="name, binary, variable"
					pageSize={10}
				/>
			</Section>

			<Section
				id="settings-gate"
				title="Human gateway"
				note={config.telegram.present ? `${config.telegram.notify.length} categories` : 'absent'}
			>
				<p class="mb-3 text-sm text-base-content/70">{configTelegramHeadline(config.telegram)}</p>
				{#if !config.telegram.present}
					<p class="text-sm text-base-content/60">
						There are no push modes to report. The gate is configured on disk, not here.
					</p>
				{:else}
					<MetaList
						label="Telegram gate"
						columns={1}
						rows={[
							{
								label: 'Enabled',
								value: config.telegram.enabled ? 'yes' : 'no'
							},
							{
								label: 'Bot token',
								value: config.telegram.botTokenSet
									? `set in $${config.telegram.botTokenEnv} or the file`
									: 'not set'
							}
						]}
					/>
					<h3 class="mt-4 mb-1 text-xs text-base-content/60">Push modes</h3>
					<!-- The seven categories are the config file's own, minus the legacy
					     `waiting_review` the server folds into the two review modes. -->
					<ul class="divide-y divide-base-200">
						{#each config.telegram.notify as entry (entry.category)}
							<DetailRow title={entry.category}>
								{#snippet side()}
									<StatusBadge
										status={entry.mode}
										label={configNotifyEnabled(entry.mode) ? entry.mode : 'off'}
									/>
								{/snippet}
							</DetailRow>
						{/each}
					</ul>
				{/if}
			</Section>

			<Section id="settings-colony" title="Colony">
				<MetaList
					label="Colony identity"
					columns={1}
					rows={[
						{ label: 'Slug', value: config.slug, mono: true, copy: true },
						{ label: 'Root', value: config.colonyRoot, mono: true, copy: true },
						{
							label: 'Profile',
							value: configProfileLabel(config),
							hint: configSourceHint(config.profile.selected)
						},
						{
							label: 'Terminal',
							// The loader's own word for the fallback is already `default`, so
							// annotating it would read "default (the default)"; the hint says it.
							value: config.terminal.terminal,
							hint: config.terminal.configured
								? []
								: ['Not written anywhere — the loader falls back to this.']
						}
					]}
				/>
			</Section>
		</div>
	{/if}
</div>
