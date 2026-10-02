<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Menu } from 'lucide-svelte';
	import { onDestroy, onMount, tick } from 'svelte';
	import Header from '$lib/components/Header.svelte';
	import SideMenu from '$lib/components/SideMenu.svelte';
	import Toast from '$lib/components/Toast.svelte';
	import {
		consolePath,
		dialogOpen,
		firstNavigationLink,
		listFilter,
		matchShortcut,
		navigationOpen,
		owningListPath,
		rememberRoute
	} from '$lib/navigation';
	import { consoleStatusStore } from '$lib/stores/console-status.svelte';
	import { sideMenuStore } from '$lib/stores/side-menu.svelte';
	import { themeStore } from '$lib/stores/theme.svelte';
	import { versionStore } from '$lib/stores/version.svelte';
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
	 * navigating out from under one would be a second, invisible way to dismiss it; and an
	 * editable target keeps it, which is the same rule that stops a chord from firing
	 * mid-word — so the half-written review note is never thrown away by a reflex, and an
	 * xterm's helper textarea swallows it before the window is even asked. The navigation
	 * sheet is the third holder, for the dialog's reason: it stands between the operator
	 * and the page, and the menu's own handler closes it.
	 */
	function handleEscape(): boolean {
		if (dialogOpen() || navigationOpen(sideMenuStore.narrow)) return false;
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

	/**
	 * The trigger opens the sheet and puts focus on the first route, because a panel that
	 * opens with focus still on the button that opened it sends the next `Tab` into the
	 * page behind it. Closing is the menu's own business — its `Escape`, the overlay, and
	 * a link — and the menu hands focus back here.
	 */
	async function toggleNavigation(): Promise<void> {
		const open = !sideMenuStore.expanded;
		sideMenuStore.set(open);
		if (!open || !sideMenuStore.narrow) return;
		await tick();
		firstNavigationLink()?.focus();
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
		sideMenuStore.hydrate();
		consoleStatusStore.start();
		// The build stamp is a constant for the life of the page, so it is read once
		// here rather than polled: the side menu's footer and the System route's build
		// block both read this one store, and neither can then describe a different
		// build than the other.
		versionStore.start();

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

<div id="console-shell" class="drawer min-h-screen bg-base-200 text-base-content md:drawer-open">
	<input
		id="side-menu-drawer"
		type="checkbox"
		class="drawer-toggle"
		data-navigation-toggle
		bind:checked={sideMenuStore.expanded}
	/>
	<div class="drawer-content min-w-0">
		<!-- `z-30` sits above the topbar's `z-20` (it is fixed over it) and below the
		     sheet's `z-40` (it must vanish under the panel it opened). -->
		<button
			id="side-menu-trigger"
			data-navigation-trigger
			type="button"
			class="btn btn-ghost btn-sm btn-square drawer-button fixed top-3 left-3 z-30 md:hidden"
			aria-label={sideMenuStore.expanded ? 'Close navigation' : 'Open navigation'}
			aria-expanded={sideMenuStore.expanded}
			aria-controls="console-navigation"
			onclick={() => void toggleNavigation()}
		>
			<Menu size={18} />
		</button>
		<Header />
		<main id="main-content" class="mx-auto w-full max-w-7xl px-4 pt-6 pb-16 md:px-6">
			{@render children()}
		</main>
	</div>
	<!-- `z-40` over daisyUI's `z-10` because the topbar here is a sticky panel with a
	     z-index of its own, and a sheet that slides in under the operator's own status
	     row is a sheet they cannot read. -->
	<div class="drawer-side z-40">
		<SideMenu />
	</div>
</div>

	<Toast />
