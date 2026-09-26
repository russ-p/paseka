<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import StatusIcon from '$lib/components/StatusIcon.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import {
		agentsDetail,
		agentsDetailFull,
		agentsMeta,
		gitDetail,
		gitLines,
		gitMeta,
		gitNeedsAttention,
		gitSyncLabel,
		hostBadge,
		hostDetail,
		hostDetailLines,
		hostLoad,
		hostMeta,
		natsLabel,
		runtimeAction,
		runtimeActionGlyph,
		runtimeActionLabel,
		runtimeDetail,
		runtimeDetailLines,
		runtimeStateNote
	} from '$lib/format';
	import {
		consoleStatusStore,
		type ConsoleStatusStore
	} from '$lib/stores/console-status.svelte';

	let {
		store = consoleStatusStore,
		toasts = toastStore
	}: { store?: ConsoleStatusStore; toasts?: ToastStore } = $props();

	let confirmStop = $state(false);
	let chooseAction = $state(false);

	const runtime = $derived(store.runtime);
	const action = $derived(runtimeAction(runtime, store.runtimeAction !== null));
	const actionLabel = $derived(runtimeActionLabel(action, store.runtimeStatus, store.runtimeAction));
	const natsWord = $derived(natsLabel(store.natsStatus));

	function requestAction(): void {
		if (action === 'stop') {
			confirmStop = true;
			return;
		}
		if (action === 'choose') {
			chooseAction = true;
			return;
		}
		if (action === 'start') {
			void handleStart();
		}
	}

	async function handleStart(): Promise<void> {
		store.clearRuntimeError();
		const ok = await store.startRuntime();
		toasts.push(ok ? 'success' : 'error', ok ? 'Hive runtime started' : store.runtimeError);
	}

	async function handleStop(): Promise<void> {
		confirmStop = false;
		store.clearRuntimeError();
		const ok = await store.stopRuntime();
		toasts.push(ok ? 'success' : 'error', ok ? 'Hive runtime stopped' : store.runtimeError);
	}
</script>

<header
		id="topbar"
		class="sticky top-0 z-30 border-b border-base-300 bg-base-100"
		aria-label="Colony status"
	>
	<div
		id="topbar-panels"
		class="flex items-stretch gap-3 overflow-x-auto px-3 py-2 md:px-6"
	>
		<div id="topbar-identity" class="flex shrink-0 items-start gap-2 self-center pr-1 pl-20">
			<div class="min-w-0">
				<p class="truncate text-base font-semibold">Queen Console</p>
				<Hint lines={[store.colony ?? 'Colony unknown']}>
					<p class="truncate text-xs text-base-content/60">{store.colony ?? 'Colony unknown'}</p>
				</Hint>
			</div>
			<StatusIcon
				status={store.natsStatus}
				label={`NATS ${natsWord}`}
				size="lg"
			/>
		</div>

		{#if store.loading}
			<span class="skeleton h-4 w-4 self-center"></span>
		{:else}
			{#if store.reviews > 0 || store.pendingSessions > 0}
				<div id="topbar-summary" class="flex shrink-0 items-center gap-2 self-center">
					{#if store.reviews > 0}
						<StatusBadge status="pending" label={`Reviews ${store.reviews}`} />
					{/if}
					{#if store.pendingSessions > 0}
						<StatusBadge status="pending" label={`Invites ${store.pendingSessions}`} />
					{/if}
				</div>
			{/if}

			<div id="topbar-blocks" class="ml-auto flex shrink-0 items-stretch gap-3">
			<section
				id="panel-runtime"
				class="w-56 shrink-0 rounded-box border border-base-300 p-2"
				aria-label="Hive runtime"
			>
				<div class="flex min-h-8 items-center justify-between gap-2">
					<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Hive runtime</span>
					<button
						id="runtime-action"
						type="button"
						class="btn btn-circle btn-ghost h-8 w-8 border border-base-300"
						class:btn-primary={action === 'start'}
						class:btn-error={action === 'stop'}
						class:btn-disabled={action === 'busy'}
						disabled={action === 'busy'}
						aria-label={actionLabel}
						title={actionLabel}
						onclick={requestAction}
					>
						<StatusIcon status={store.runtimeStatus} glyph={runtimeActionGlyph(action)} size="md" />
					</button>
				</div>
				<Hint lines={[runtimeStateNote(runtime)]}>
					<p class="truncate text-xs text-base-content/70">
						{store.runtimeAction === 'starting'
							? 'starting…'
							: store.runtimeAction === 'stopping'
								? 'stopping…'
								: runtimeStateNote(runtime)}
					</p>
				</Hint>
				{#if runtimeDetailLines(runtime).length > 0}
					<Hint lines={runtimeDetailLines(runtime)}>
						<p class="truncate text-xs text-base-content/50">{runtimeDetail(runtime)}</p>
					</Hint>
				{/if}
			</section>

			<section
				id="panel-bees"
				class="w-48 shrink-0 rounded-box border border-base-300 p-2"
				aria-label="Live bees"
			>
				<div class="flex min-h-8 items-center justify-between gap-2">
					<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Live bees</span>
					<StatusBadge status={store.liveBees > 0 ? 'live' : 'idle'} label={String(store.liveBees)} />
				</div>
				<Hint lines={[agentsMeta(store.agents)]}>
					<p class="truncate text-xs text-base-content/70">{agentsMeta(store.agents)}</p>
				</Hint>
				{#if agentsDetailFull(store.agents?.items)}
					<Hint lines={agentsDetailFull(store.agents?.items).split(' · ')}>
						<p class="truncate text-xs text-base-content/50">
							{agentsDetail(store.agents?.items)}
						</p>
					</Hint>
				{/if}
			</section>

			<section
				id="panel-host"
				class="w-48 shrink-0 rounded-box border border-base-300 p-2"
				aria-label="Host"
			>
				<div class="flex min-h-8 items-center justify-between gap-2">
					<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Host</span>
					<StatusBadge status={store.host ? 'live' : 'unavailable'} label={hostBadge(store.host)} />
				</div>
				<Hint lines={hostDetailLines(store.host, store.hostError)}>
					<p class="truncate text-xs text-base-content/70">
						{hostMeta(store.host, store.hostError)}
					</p>
				</Hint>
				{#if hostLoad(store.host)}
					<Hint lines={hostDetail(store.host).split(' · ')}>
						<p class="truncate text-xs text-base-content/50">{hostLoad(store.host)}</p>
					</Hint>
				{/if}
			</section>

			<section
				id="panel-git"
				class="w-48 shrink-0 rounded-box border border-base-300 p-2"
				aria-label="Git"
			>
				<div class="flex min-h-8 items-center justify-between gap-2">
					<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Git</span>
					<StatusBadge
						status={gitNeedsAttention(store.git) ? 'pending' : 'success'}
						label={gitSyncLabel(store.git)}
					/>
				</div>
				<Hint lines={gitLines(store.git, store.gitError)}>
					<p class="truncate text-xs text-base-content/70">
						{gitMeta(store.git, store.gitError)}
					</p>
				</Hint>
				{#if gitDetail(store.git, store.gitError)}
					<Hint lines={gitDetail(store.git, store.gitError).split(' · ')}>
						<p class="truncate text-xs text-base-content/50">
							{gitDetail(store.git, store.gitError)}
						</p>
					</Hint>
				{/if}
			</section>
			</div>
		{/if}
	</div>

	{#if store.runtimeError}
		<div id="topbar-runtime-error" class="alert alert-error mx-3 my-2 md:mx-6" role="alert">
			<span>{store.runtimeError}</span>
		</div>
	{/if}

	{#if store.connection !== 'connected'}
		<div
			id="topbar-connection-banner"
			class="flex items-center gap-2 border-t border-warning/20 bg-warning/10 px-4 py-2"
			role="status"
		>
			<StatusBadge status={store.connection} />
			<span class="text-sm">
				{store.connection === 'reconnecting'
					? 'Reconnecting to the console event stream.'
					: 'The console event stream is unavailable.'}
			</span>
		</div>
	{/if}

	{#if store.lastError}
		<div id="topbar-stream-error" class="alert alert-error mx-3 my-2 md:mx-6" role="alert">
			<span>{store.lastError}</span>
		</div>
	{/if}
</header>

<Modal
	open={chooseAction}
	title="Hive runtime state is unclear"
	description="The console cannot tell whether the hive is running. Choose what to do."
	onclose={() => (chooseAction = false)}
>
	<p class="text-sm text-base-content/70">
		Reported status: <span class="font-semibold">{store.runtimeStatus}</span>
	</p>
	{#snippet footer()}
		<button type="button" class="btn btn-ghost btn-sm" onclick={() => (chooseAction = false)}>
			Cancel
		</button>
		<button
			type="button"
			class="btn btn-error btn-sm"
			onclick={() => {
				chooseAction = false;
				confirmStop = true;
			}}
		>
			Stop…
		</button>
		<button
			id="runtime-choose-start"
			type="button"
			class="btn btn-primary btn-sm"
			onclick={() => {
				chooseAction = false;
				void handleStart();
			}}
		>
			Start
		</button>
	{/snippet}
</Modal>

<Modal
	open={confirmStop}
	title="Stop the hive runtime?"
	description="Active agents lose their NATS connection and unfinished runs are left behind. Operator input already submitted is not affected."
	onclose={() => (confirmStop = false)}
>
	<p class="text-sm text-base-content/70">
		Current status: <span class="font-semibold">{store.runtimeStatus}</span>
	</p>
	{#snippet footer()}
		<button
			id="runtime-stop-cancel"
			type="button"
			class="btn btn-ghost btn-sm"
			onclick={() => (confirmStop = false)}
		>
			Cancel
		</button>
		<button id="runtime-stop-confirm" type="button" class="btn btn-error btn-sm" onclick={() => void handleStop()}>
			Stop runtime
		</button>
	{/snippet}
</Modal>
