<script lang="ts">
	import '../app.css';
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="min-h-screen bg-base-200 text-base-content">
	<main class="mx-auto grid min-h-screen w-full max-w-7xl place-items-center p-6">
		{@render children()}
	</main>
</div>
