<script lang="ts">
	import BeeRunDrawer from '$lib/components/BeeRunDrawer.svelte';
	import type { Bee } from '$lib/api/types';

	let { roster }: { roster: Bee[] } = $props();

	let open = $state(false);
	/**
	 * Stands in for the page behind the drawer re-reading its own roster — a poll, a
	 * Refresh, a commit landing — while the form is open. Rerendering the component
	 * with new props cannot express that: testing-library replaces the whole props
	 * object, so *every* effect re-runs whatever it tracked, which is a harness
	 * artifact rather than what a parent component does.
	 */
	let grown = $state<Bee[]>([]);

	function regrow(): void {
		grown = [...roster, { ...roster[0], role: 'drone' } as Bee];
	}
</script>

<button type="button" onclick={() => (open = true)}>Open</button>
<button type="button" onclick={regrow}>Re-read bees</button>

<BeeRunDrawer
	{open}
	bees={grown.length > 0 ? grown : roster}
	traceId="trail-daily"
	onclose={() => (open = false)}
/>