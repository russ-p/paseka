<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import Header from '$lib/components/Header.svelte';
	import SideMenu from '$lib/components/SideMenu.svelte';
	import { consolePath, matchShortcut, rememberRoute } from '$lib/navigation';
	import { consoleStatusStore } from '$lib/stores/console-status.svelte';
	import { themeStore } from '$lib/stores/theme.svelte';
	import type { Snippet } from 'svelte';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	let chordPending = false;
	let chordTimer: ReturnType<typeof setTimeout> | undefined;

	function editableTarget(target: EventTarget | null): boolean {
		if (!(target instanceof HTMLElement)) return false;
		return target.isContentEditable || ['INPUT', 'SELECT', 'TEXTAREA'].includes(target.tagName);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (editableTarget(event.target)) return;
		const key = event.key.toLowerCase();

		const match = matchShortcut(chordPending, key);
		chordPending = match.pending;
		if (chordTimer) clearTimeout(chordTimer);
		if (!match.path) {
			if (match.pending) {
				chordTimer = setTimeout(() => {
					chordPending = false;
				}, 1200);
			}
			return;
		}
		event.preventDefault();
		void goto(consolePath(base, match.path));
	}

	onMount(() => {
		themeStore.hydrate();
		consoleStatusStore.start();

		const handleVisibility = () => {
			if (document.hidden) consoleStatusStore.stop();
			else consoleStatusStore.start();
		};
		document.addEventListener('visibilitychange', handleVisibility);

		return () => {
			document.removeEventListener('visibilitychange', handleVisibility);
			consoleStatusStore.stop();
		};
	});

	onDestroy(() => {
		if (chordTimer) clearTimeout(chordTimer);
	});

	$effect(() => {
		rememberRoute(base, page.url.pathname);
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<a
	href="#main-content"
	class="btn btn-primary btn-sm fixed top-3 left-1/2 z-50 -translate-x-1/2 -translate-y-20 focus:translate-y-0"
>
	Skip to content
</a>

<div class="min-h-screen bg-base-200 text-base-content">
	<Header />
	<SideMenu />
	<main id="main-content" class="mx-auto w-full max-w-7xl px-4 pt-6 pb-16 md:px-6">
		{@render children()}
	</main>
</div>
