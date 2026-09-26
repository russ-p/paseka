<script lang="ts">
	import Drawer from '$lib/components/Drawer.svelte';
	import { createTask, listBees } from '$lib/api/client';
	import type { Bee, CreateTaskResult } from '$lib/api/types';

	let {
		open,
		onclose,
		oncreated
	}: {
		open: boolean;
		onclose: () => void;
		/** Fired with the created task so the caller can refresh, toast, and link to it. */
		oncreated?: (result: CreateTaskResult) => void;
	} = $props();

	let bees = $state<Bee[]>([]);
	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');

	let title = $state('');
	let body = $state('');
	let bee = $state('');
	let traceId = $state('');
	let sector = $state('');
	let intent = $state('');
	let review = $state('none');
	let dependsOn = $state('');
	let autorun = $state(false);

	/** The server accepts a title or a body, so the hint says that rather than lying. */
	const empty = $derived(title.trim() === '' && body.trim() === '');
	/** A task with no bee has nobody to dispatch to, so unlike the server this is required. */
	const ready = $derived(!empty && bee !== '' && !submitting);
	const selectedBee = $derived(bees.find((entry) => entry.role === bee));

	/**
	 * The bee's own prompt templates are the intents it can be launched under, so
	 * the list comes from the colony rather than from a constant here — a bee that
	 * adds an intent needs no console change.
	 */
	const intents = $derived(
		selectedBee ? (selectedBee.defaultIntent && !selectedBee.intents.includes(selectedBee.defaultIntent) ? [...selectedBee.intents, selectedBee.defaultIntent] : selectedBee.intents) : []
	);

	$effect(() => {
		if (!open) return;
		title = '';
		body = '';
		bee = '';
		traceId = '';
		sector = '';
		intent = '';
		review = 'none';
		dependsOn = '';
		autorun = false;
		error = '';
		void load();
	});

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			bees = await listBees();
			if (bees.length === 1) selectBee(bees[0].role);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			loading = false;
		}
	}

	/** Default the intent to the bee's own, so the common case is one click. */
	function selectBee(role: string): void {
		bee = role;
		const entry = bees.find((candidate) => candidate.role === role);
		intent = entry?.defaultIntent ?? '';
	}

	async function submit(): Promise<void> {
		if (!ready) return;
		submitting = true;
		error = '';
		try {
			const result = await createTask({
				title: title.trim(),
				body: body.trim(),
				bee,
				// An empty optional field is left out rather than sent as `""`: the
				// server treats a present-but-blank trace id differently from an absent
				// one, where absent means "generate one".
				...(traceId.trim() === '' ? {} : { traceId: traceId.trim() }),
				...(sector.trim() === '' ? {} : { sector: sector.trim() }),
				...(intent === '' ? {} : { intent }),
				...(dependsOn.trim() === '' ? {} : { dependsOn: splitIds(dependsOn) }),
				review,
				autorun
			});
			oncreated?.(result);
			onclose();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = false;
		}
	}

	/** Comma or whitespace separated, because that is how a dependency list is read. */
	function splitIds(value: string): string[] {
		return value
			.split(/[\s,]+/)
			.map((part) => part.trim())
			.filter((part) => part !== '');
	}
</script>

<Drawer
	{open}
	title="New task"
	description="Hand a unit of work to a bee. The task joins the trail below it in the board."
	onclose={onclose}
>
	{#if error}
		<div class="alert alert-error mb-3" role="alert"><span>{error}</span></div>
	{/if}

	{#if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
		</div>
	{:else if bees.length === 0}
		<div class="alert alert-warning" role="alert">
			<span>
				No launchable bees. <code class="font-mono">GET /api/bees</code> lists the bees whose
				adapter supports interactive sessions, so a colony of script adapters has none — the
				task is still creatable through <code class="font-mono">paseka task create</code>.
			</span>
		</div>
	{:else}
		<div class="space-y-4">
			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-title">Title</label>
				<input
					id="task-create-title"
					type="text"
					class="input input-bordered w-full"
					placeholder="What the bee is asked to do"
					bind:value={title}
				/>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-body">Body</label>
				<textarea
					id="task-create-body"
					class="textarea textarea-bordered min-h-32 w-full"
					placeholder="The full prompt: context, constraints, what done looks like"
					bind:value={body}
				></textarea>
				<span id="task-create-body-hint" class="fieldset-label">
					{empty ? 'A title or a body is required.' : 'What the bee is handed, verbatim.'}
				</span>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-bee">Bee</label>
				<select
					id="task-create-bee"
					class="select select-bordered w-full"
					bind:value={bee}
					onchange={() => selectBee(bee)}
				>
					<option value="" disabled>Select a bee</option>
					{#each bees as entry (entry.role)}
						<option value={entry.role}>{entry.role} — {entry.adapter}</option>
					{/each}
				</select>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-intent">Intent</label>
				<select
					id="task-create-intent"
					class="select select-bordered w-full"
					disabled={intents.length === 0}
					bind:value={intent}
				>
					<option value="">None</option>
					{#each intents as name (name)}
						<option value={name}>{name}</option>
					{/each}
				</select>
				<span class="fieldset-label">
					{intents.length === 0
						? 'The selected bee declares no intents.'
						: `Prompt templates ${selectedBee?.role ?? ''} declares.`}
				</span>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-trace">Trail ID</label>
				<input
					id="task-create-trace"
					type="text"
					class="input input-bordered w-full font-mono"
					placeholder="auto-generated"
					bind:value={traceId}
				/>
				<span class="fieldset-label">
					Leave empty for a new trail, or name a standing one to add this task to it. Left empty,
					both this id and the task id are generated server-side.
				</span>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-sector">Sector</label>
				<input
					id="task-create-sector"
					type="text"
					class="input input-bordered w-full"
					placeholder="from colony.yaml"
					bind:value={sector}
				/>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-review">Review</label>
				<select id="task-create-review" class="select select-bordered w-full" bind:value={review}>
					<option value="none">Not gated</option>
					<option value="required">Review required</option>
					<option value="final">Final gate</option>
				</select>
				<span class="fieldset-label">
					{#if review === 'none'}
						The bee finishes and the work is yours.
					{:else if review === 'final'}
						The last thing to leave the colony needs your sign-off, and the whole trail waits on it.
					{:else}
						The bee's work stops for your sign-off before it counts.
					{/if}
				</span>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-create-depends">Depends on</label>
				<input
					id="task-create-depends"
					type="text"
					class="input input-bordered w-full font-mono"
					placeholder="task-01, task-02"
					bind:value={dependsOn}
				/>
				<span class="fieldset-label">
					These tasks must complete first. A blocked dependency keeps this one from starting.
				</span>
			</div>

			<label class="flex cursor-pointer items-center gap-2">
				<input type="checkbox" class="checkbox checkbox-sm" bind:checked={autorun} />
				<span class="label">Start immediately</span>
			</label>
			<span class="-mt-2 text-xs text-base-content/60">
				Publishes <code class="font-mono">task.ready</code> so a dispatcher picks it up without a
				trip through the board. Creating and starting both need NATS and a running
				<code class="font-mono">paseka run</code>.
			</span>
		</div>
	{/if}

	{#snippet footer()}
		<div class="flex items-center gap-2">
			<button type="button" class="btn btn-ghost btn-sm" onclick={onclose}>Cancel</button>
			<button
				type="button"
				class="btn btn-primary btn-sm"
				disabled={!ready}
				onclick={() => void submit()}
			>
				{submitting ? 'Creating…' : autorun ? 'Create and start' : 'Create'}
			</button>
		</div>
	{/snippet}
</Drawer>
