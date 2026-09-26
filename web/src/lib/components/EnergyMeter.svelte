<script lang="ts">
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import {
		energyAvailable,
		energyDenominator,
		energyLabel,
		energyLowLabel,
		energyMetaLabel
	} from '$lib/format';
	import type { HoneyReserve } from '$lib/api/types';

	let {
		energy,
		pending = false,
		error = '',
		ontopup
	}: {
		energy: HoneyReserve;
		/** A top-up is in flight; the buttons are disabled so no two overlap. */
		pending?: boolean;
		error?: string;
		ontopup: (amount: number) => void;
	} = $props();

	/** The console's fixed top-up steps; the server accepts any positive amount. */
	const steps = [1, 5, 12];

	const denominator = $derived(energyDenominator(energy));
	const remaining = $derived(energy.energyRemaining ?? 0);
	const available = $derived(energyAvailable(energy));
	const low = $derived(energyLowLabel(energy));
	const meta = $derived(energyMetaLabel(energy));
</script>

<div class="space-y-2">
	{#if available}
		<div class="flex flex-wrap items-center justify-between gap-2">
			<div class="flex min-w-0 items-center gap-2">
				<span class="text-lg font-bold">{energyLabel(energy)}</span>
				{#if low}
					<StatusBadge status="low" label={low} />
				{/if}
			</div>
			<div class="join" role="group" aria-label="Top up the honey reserve">
				{#each steps as step (step)}
					<button
						type="button"
						class="btn btn-sm join-item"
						disabled={pending}
						onclick={() => ontopup(step)}
					>
						+{step}
					</button>
				{/each}
			</div>
		</div>
		<progress
			class="progress progress-primary w-full"
			value={Math.min(remaining, denominator > 0 ? denominator : remaining)}
			max={denominator > 0 ? denominator : remaining}
		></progress>
		{#if meta}
			<p class="text-xs text-base-content/50">{meta}</p>
		{/if}
	{:else}
		<p class="text-sm text-base-content/60">
			Honey reserve unavailable — top up when NATS and the task ledger are connected.
		</p>
	{/if}
	{#if error}
		<p class="text-sm text-error" role="alert">{error}</p>
	{/if}
</div>
