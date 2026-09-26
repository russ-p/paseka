<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { consolePath, consoleRoutes } from '$lib/navigation';
	import { tick } from 'svelte';

	let { currentPath = page.url.pathname }: { currentPath?: string } = $props();

	let open = $state(false);
	let trigger = $state<HTMLButtonElement | null>(null);
	let panel = $state<HTMLElement | null>(null);

	const panelClass = $derived(
		`fixed inset-x-0 bottom-0 z-40 h-80 w-full border-t border-base-300 bg-base-100 shadow-2xl transition-transform md:inset-y-0 md:left-0 md:right-auto md:h-full md:w-72 md:border-t-0 md:border-r ${open ? 'translate-x-0 translate-y-0' : '-translate-x-full translate-y-full md:translate-y-0'}`
	);

	function focusableElements(): HTMLElement[] {
		if (!panel) return [];
		return Array.from(panel.querySelectorAll<HTMLElement>('a[href], button:not([disabled])'));
	}

	async function openMenu(): Promise<void> {
		open = true;
		await tick();
		panel?.querySelector<HTMLElement>('a[href]')?.focus();
	}

	async function closeMenu(restoreFocus = true): Promise<void> {
		open = false;
		await tick();
		if (restoreFocus) trigger?.focus();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!open) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			void closeMenu();
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
</script>

<svelte:window onkeydown={handleKeydown} />

<button
	bind:this={trigger}
	id="side-menu-trigger"
	type="button"
	class="btn btn-ghost btn-sm fixed top-3 left-3 z-50"
	aria-label="Menu"
	aria-expanded={open}
	aria-controls="console-navigation"
	onclick={() => (open ? void closeMenu() : void openMenu())}
>
	Menu
</button>

{#if open}
	<button
		id="side-menu-backdrop"
		type="button"
		class="fixed inset-0 z-30 cursor-default bg-neutral/40"
		aria-label="Close navigation"
		onclick={() => void closeMenu()}
	></button>
{/if}

<aside
	bind:this={panel}
	id="console-navigation"
	class={panelClass}
	data-state={open ? 'open' : 'closed'}
	aria-label="Console navigation"
	aria-hidden={!open}
	inert={!open}
>
	<div class="flex h-full flex-col">
			<div class="flex items-center justify-between border-b border-base-300 p-4 md:pl-20">
			<div>
				<p class="font-semibold">Navigate</p>
				<p class="text-xs text-base-content/60">Queen Console Next</p>
			</div>
			<button id="side-menu-close" type="button" class="btn btn-ghost btn-sm" onclick={() => void closeMenu()}>
				Close
			</button>
		</div>

		<ul class="menu w-full grow flex-nowrap gap-1 overflow-y-auto p-3">
			{#each consoleRoutes as route, index (route.path)}
				{@const href = consolePath(base, route.path)}
				{@const group = consoleRoutes[index - 1]?.group}
				{#if route.group !== group}
					<li class="menu-title">{route.group}</li>
				{/if}
				<li>
					<a
						{href}
						class={currentPath === href ? 'menu-active' : ''}
						aria-current={currentPath === href ? 'page' : undefined}
						onclick={() => void closeMenu()}
					>
						<span>{route.label}</span>
						<kbd class="kbd kbd-sm w-fit justify-self-end">g {route.shortcut}</kbd>
					</a>
				</li>
			{/each}
		</ul>

		<div class="border-t border-base-300 p-4">
			<a id="side-menu-legacy" class="btn btn-ghost btn-sm w-full" href="/">Open legacy console</a>
		</div>
	</div>
</aside>
