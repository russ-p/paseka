<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import Header from '$lib/components/Header.svelte';
	import SideMenu from '$lib/components/SideMenu.svelte';
	import Toast from '$lib/components/Toast.svelte';
	import { consolePath, dialogOpen, listFilter, matchShortcut, owningListPath, rememberRoute } from '$lib/navigation';
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

	/**
	 * `Escape` returns to the list a detail belongs to, which is the way out of a trail,
	 * a run, a task, a proposal or a session that has no breadcrumb of its own. Two things
	 * it deliberately does not do: a dialog keeps it, because a dialog closes itself and
	 * navigating out from under one would be a second, invisible way to dismiss it; and
	 * an editable target keeps it, which is the same rule that stops a chord from firing
	 * mid-word — so the half-written review note is never thrown away by a reflex, and an
	 * xterm's helper textarea swallows it before the window is even asked.
	 */
	function handleEscape(): boolean {
		if (dialogOpen()) return false;
		const list = owningListPath(base, page.url.pathname);
		if (!list) return false;
		void goto(list);
		return true;
	}

	/**
	 * `/` reaches the list's filter, the way it does in every tool an operator already
	 * has open next to this one. It selects what is there, because arriving at a filter
	 * you meant to replace is the normal reason to press it; a page with no list has
	 * nothing to focus and says nothing.
	 */
	function handleSlash(): boolean {
		const filter = listFilter();
		if (!filter) return false;
		filter.focus();
		filter.select();
		return true;
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (editableTarget(event.target)) return;
		const key = event.key.toLowerCase();

		if (key === 'escape' && handleEscape()) {
			event.preventDefault();
			return;
		}
		if (key === '/' && handleSlash()) {
			// Without this the slash lands in the filter, which now holds the very
			// character that was used to get there.
			event.preventDefault();
			return;
		}

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
	id="skip-to-content"
	href="#main-content"
	class="btn btn-primary btn-sm fixed top-3 left-1/2 z-50 -translate-x-1/2 -translate-y-20 focus:translate-y-0"
>
	Skip to content
</a>

<div id="console-shell" class="min-h-screen bg-base-200 text-base-content">
	<Header />
	<SideMenu />
	<main id="main-content" class="mx-auto w-full max-w-7xl px-4 pt-6 pb-16 md:px-6">
		{@render children()}
	</main>
</div>

	<Toast />
