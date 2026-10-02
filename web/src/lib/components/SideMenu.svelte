<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { ExternalLink, PanelLeftClose, PanelLeftOpen, X } from 'lucide-svelte';
	import {
		consolePath,
		consoleRoutes,
		isRouteActive,
		navigationTrigger
	} from '$lib/navigation';
	import { routeGlyphs } from '$lib/route-icons';
	import { sideMenuStore, type SideMenuStore } from '$lib/stores/side-menu.svelte';
	import { versionStamp, versionTooltip } from '$lib/format';
	import { versionStore, type VersionStore } from '$lib/stores/version.svelte';
	import { tick } from 'svelte';

	let {
		store = sideMenuStore,
		currentPath = page.url.pathname,
		version = versionStore
	}: { store?: SideMenuStore; currentPath?: string; version?: VersionStore } = $props();

	let panel = $state<HTMLElement | null>(null);

	const expanded = $derived(store.expanded);

	/**
	 * The sheet is the only state of this drawer that is modal, so it is the only one that
	 * owns `Escape` and traps `Tab`. Above 768px the panel is a column in the layout with
	 * the page beside it: a trap there would make the page unreachable, and `Escape` must
	 * still walk a trail detail back to its list.
	 */
	const sheet = $derived(store.narrow && store.expanded);

	/**
	 * Labels are the default, not a reward for the expanded state: the sheet below 768px
	 * has the whole width and no second control to collapse it with, so a phone showing a
	 * rail of unlabelled icons would be the one place the menu cannot be read at all.
	 */
	const labelled = $derived(expanded || store.narrow);

	async function closeSheet(): Promise<void> {
		store.set(false);
		await tick();
		navigationTrigger()?.focus();
	}

	/**
	 * One control in one place, doing the only thing that is available where it is: the
	 * rail toggle above the breakpoint, and the sheet's close below it — where the sheet
	 * covers the topbar's own trigger, so the panel has to be able to dismiss itself. It is
	 * worded apart from the trigger (`Close menu` against `Close navigation`) because two
	 * controls with one accessible name in one view is a name the operator cannot act on.
	 * A close of its own also keeps the panel free of a control that is hidden at one
	 * width, which would leave the `Tab` trap wrapping onto something nobody can see.
	 */
	const headAction = $derived(store.narrow ? 'close' : 'toggle');
	const headLabel = $derived(
		headAction === 'close' ? 'Close menu' : expanded ? 'Hide route labels' : 'Show route labels'
	);

	function focusableElements(): HTMLElement[] {
		if (!panel) return [];
		return Array.from(panel.querySelectorAll<HTMLElement>('a[href], button:not([disabled])'));
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!sheet) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			void closeSheet();
			return;
		}
		if (event.key !== 'Tab') return;

		const elements = focusableElements();
		if (elements.length === 0) return;
		const first = elements[0];
		const last = elements[elements.length - 1];
		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}

	function onHead(): void {
		if (headAction === 'close') {
			void closeSheet();
			return;
		}
		store.set(!expanded);
	}

	/**
	 * A link closes the sheet, because the sheet covers the page it navigates to. It does
	 * not touch the flag above the breakpoint, where the panel stays put and the same flag
	 * means the labels — a link that collapsed the rail would make the column disappear on
	 * every navigation, which is the one thing a persistent menu cannot do.
	 */
	function onNavigate(): void {
		if (store.narrow) store.set(false);
	}

	const stamp = $derived(versionStamp(version.build, labelled));
	const tooltip = $derived(versionTooltip(version.build));
	// The commit when the build knows one, the repository when it does not: an
	// unstamped build has no commit to show, and the repository is still the better
	// answer than a link nowhere.
	const stampHref = $derived(version.build?.commitUrl ?? version.build?.repository ?? '');
</script>

<svelte:window onkeydown={handleKeydown} />

<label for="side-menu-drawer" class="drawer-overlay" aria-label="Close navigation"></label>

<aside
	bind:this={panel}
	id="console-navigation"
	class="flex h-full flex-col bg-base-100 md:border-r md:border-base-300"
	class:w-56={labelled}
	class:w-16={!labelled}
	data-state={expanded ? 'expanded' : 'mini'}
	aria-label="Console navigation"
>
	<div
		class="border-b border-base-300 p-3 {labelled
			? 'flex items-center justify-between gap-2'
			: 'flex flex-col items-center gap-2'}"
	>
		<div class="flex min-w-0 items-center gap-2">
			<img src="/favicon.svg" alt="" class="size-7 shrink-0 rounded-md" />
			{#if labelled}
				<!-- The panel's subject, not the app's name: the topbar's identity block
				     already says which console this is, and a side menu that repeats it
				     reads as two headings for one thing. -->
				<p class="truncate text-sm font-semibold">Navigate</p>
			{/if}
		</div>
		<button
			id="side-menu-collapse"
			type="button"
			class="btn btn-ghost btn-sm btn-square"
			class:mx-auto={!labelled}
			class:ml-auto={labelled}
			aria-label={headLabel}
			aria-pressed={headAction === 'toggle' ? expanded : undefined}
			onclick={onHead}
		>
			{#if headAction === 'close'}
				<X size={16} />
			{:else if expanded}
				<PanelLeftClose size={16} />
			{:else}
				<PanelLeftOpen size={16} />
			{/if}
		</button>
	</div>

	<ul class="menu w-full grow flex-nowrap gap-1 overflow-y-auto p-2">
		{#each consoleRoutes as route, index (route.path)}
			{@const href = consolePath(base, route.path)}
			{@const group = consoleRoutes[index - 1]?.group}
			{@const active = isRouteActive(href, currentPath)}
			{@const Glyph = routeGlyphs[route.glyph]}
			{#if route.group !== group}
				{#if labelled}
					<li class="menu-title">{route.group}</li>
				{:else}
					<li class="menu-title" aria-hidden="true">
						<span class="mx-auto block h-px w-5 bg-base-300"></span>
					</li>
				{/if}
			{/if}
			<li>
				<a
					{href}
					data-navigation-link
					class={active ? 'menu-active' : ''}
					class:justify-center={!labelled}
					class:justify-start={labelled}
					aria-current={active ? 'page' : undefined}
					aria-label={route.label}
					title={labelled ? undefined : route.label}
					onclick={onNavigate}
				>
					<Glyph size={18} class="shrink-0" data-glyph={route.glyph} />
					{#if labelled}
						<span class="truncate">{route.label}</span>
						<kbd class="kbd kbd-sm w-fit justify-self-end">g {route.shortcut}</kbd>
					{/if}
				</a>
			</li>
		{/each}
	</ul>

	<div class="border-t border-base-300 p-2">
		<!--
			Which build this console is, as a link to it. The stamp sits in the panel's
			foot because it is the one fact an operator needs without navigating
			anywhere: the same version a bug report has to name. Making the stamp itself
			the link is the point — a sha with nowhere to go is half an answer, and a
			second button beside it would claim a row of the one part of the panel that
			scrolls, hiding a route to save a control.

			The href comes from the server rather than the bundle: a console built from a
			fork must offer that fork, not upstream. It is the commit when there is one
			and the repository root when there is not, because an unstamped build has no
			commit to show and the repository is still the better answer.
		-->
		{#if stamp}
			<a
				id="side-menu-version"
				class="block truncate px-2 pb-1 text-left font-mono text-[10px] leading-tight text-base-content/50 hover:text-base-content"
				href={stampHref}
				target="_blank"
				rel="noopener noreferrer"
				aria-label={`${stamp} — open on GitHub`}
				title={tooltip}
			>
				{stamp}
			</a>
		{/if}
		<a
			id="side-menu-legacy"
			class="btn btn-ghost btn-sm w-full"
			class:btn-square={!labelled}
			href="/"
			aria-label="Open legacy console"
			title={labelled ? undefined : 'Open legacy console'}
		>
			{#if labelled}
				Open legacy console
			{:else}
				<ExternalLink size={16} />
			{/if}
		</a>
	</div>
</aside>
