import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { tick } from 'svelte';
import { describe, expect, it } from 'vitest';
import SideMenu from './SideMenu.svelte';
import { consoleRoutes } from '$lib/navigation';
import { createSideMenuStore, type SideMenuStorage, type SideMenuViewport } from '$lib/stores/side-menu.svelte';

function memoryStorage(initial: Record<string, string> = {}): SideMenuStorage & {
	values: Record<string, string>;
} {
	const values = { ...initial };
	return {
		values,
		getItem: (key: string) => values[key] ?? null,
		setItem: (key: string, value: string) => {
			values[key] = value;
		}
	};
}

function fixedViewport(narrow: boolean): SideMenuViewport & { emit: (narrow: boolean) => void } {
	let onChange: (narrow: boolean) => void = () => {};
	return {
		get narrow() {
			return narrow;
		},
		watch(handler: (narrow: boolean) => void) {
			onChange = handler;
		},
		emit: (value: boolean) => onChange(value)
	};
}

function menuAt(
	currentPath: string,
	options: { stored?: Record<string, string>; narrow?: boolean } = {}
) {
	const storage = memoryStorage(options.stored ?? {});
	const viewport = fixedViewport(options.narrow ?? false);
	const store = createSideMenuStore(storage, viewport);
	const view = render(SideMenu, { store, currentPath });
	return { storage, viewport, store, ...view };
}

/** The trigger lives in the shell, so the menu hands focus back to it by asking the DOM. */
function triggerOnScreen(): HTMLButtonElement {
	const trigger = document.createElement('button');
	trigger.dataset.navigationTrigger = '';
	document.body.appendChild(trigger);
	return trigger;
}

/**
 * A route link in a suite is a real `<a href>`, and jsdom answers the click with "Not
 * implemented: navigation to another Document" — there is no client router here to take
 * it. Preventing the default for the duration is what the router does in the app, and it
 * leaves the menu's own handler to run.
 */
async function clickRoute(user: ReturnType<typeof userEvent.setup>, name: string): Promise<void> {
	const swallow = (event: Event) => event.preventDefault();
	document.addEventListener('click', swallow);
	try {
		await user.click(screen.getByRole('link', { name }));
	} finally {
		document.removeEventListener('click', swallow);
	}
}

describe('SideMenu', () => {
	it('names every route and draws it, which is what the icons-only state leaves behind', async () => {
		const { container } = menuAt('/next/dashboard', { stored: { 'paseka:console:side-menu-expanded': 'mini' } });

		const links = screen.getAllByRole('link');
		for (const route of consoleRoutes) {
			expect(screen.getByRole('link', { name: route.label })).toHaveAttribute('href', `/next${route.path}`);
		}
		expect(container.querySelectorAll('svg[data-glyph]')).toHaveLength(consoleRoutes.length);
		expect(links.every((link) => link.getAttribute('title') !== null)).toBe(true);
	});

	it('carries the app mark in the head, and names the panel rather than the console', async () => {
		const { container, store } = menuAt('/next/dashboard');

		const head = container.querySelector('#console-navigation > div');
		expect(head?.querySelector('img')?.getAttribute('src')).toBe('/favicon.svg');
		expect(screen.getByText('Navigate')).toBeInTheDocument();
		// The topbar's identity block already says which console this is, and a side menu
		// that repeats it reads as two headings for one thing.
		expect(screen.queryByText('Queen Console')).toBeNull();

		store.set(false);
		await waitFor(() => expect(screen.queryByText('Navigate')).toBeNull());
		expect(container.querySelector('#console-navigation > div')?.querySelector('img')).not.toBeNull();
	});

	it('swaps one button between the two states and says which it offers', async () => {
		const user = userEvent.setup();
		const { store } = menuAt('/next/dashboard');
		const collapse = screen.getByRole('button', { name: 'Hide route labels' });

		expect(collapse).toHaveAttribute('aria-pressed', 'true');

		await user.click(collapse);

		expect(store.expanded).toBe(false);
		const offer = screen.getByRole('button', { name: 'Show route labels' });
		expect(offer).toHaveAttribute('aria-pressed', 'false');

		await user.click(offer);

		expect(store.expanded).toBe(true);
	});

	it('shows the label and the chord when expanded, and neither when it does not', async () => {
		const { container, store } = menuAt('/next/dashboard');

		expect(screen.getByText('g d')).toBeInTheDocument();
		expect(container.querySelector('#console-navigation')?.className).toContain('w-56');

		store.set(false);

		await waitFor(() => expect(screen.queryByText('g d')).toBeNull());
		expect(container.querySelector('#console-navigation')?.className).toContain('w-16');
		expect(container.querySelector('#console-navigation')?.getAttribute('data-state')).toBe('mini');
	});
});
describe('SideMenu active route', () => {
	it('keeps a parent entry active on a child route', async () => {
		menuAt('/next/traces/trace-01a0bd6963faa14f');

		expect(screen.getByRole('link', { name: 'Traces' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'Dashboard' })).not.toHaveAttribute('aria-current');
	});

	it('does not mistake a sibling route with a shared prefix for the entry', async () => {
		menuAt('/next/timeline');

		expect(screen.getByRole('link', { name: 'Traces' })).not.toHaveAttribute('aria-current');
		expect(screen.getByRole('link', { name: 'Timeline' })).toHaveAttribute('aria-current', 'page');
	});
});

describe('SideMenu as the sheet below 768px', () => {
	/** Open the sheet the way the trigger does, and let the DOM catch up. */
	async function sheetAt(currentPath: string) {
		const made = menuAt(currentPath, { narrow: true });
		made.store.set(true);
		await tick();
		return made;
	}

	it('closes on Escape and hands focus back to the trigger', async () => {
		const user = userEvent.setup();
		const trigger = triggerOnScreen();
		const { store } = await sheetAt('/next/dashboard');

		await user.keyboard('{Escape}');

		expect(store.expanded).toBe(false);
		expect(trigger).toHaveFocus();
	});

	it('closes on a route click, because the sheet covers the route it opens', async () => {
		const user = userEvent.setup();
		const { store } = await sheetAt('/next/dashboard');

		await clickRoute(user, 'Traces');

		expect(store.expanded).toBe(false);
	});

	it('keeps Tab inside the sheet, wrapping through its own controls', async () => {
		const user = userEvent.setup();
		await sheetAt('/next/dashboard');

		// The panel's own control is the first thing in it, and the legacy link the last,
		// so those are the two ends a Tab has to wrap between.
		const close = screen.getByRole('button', { name: 'Close menu' });
		const legacy = screen.getByRole('link', { name: 'Open legacy console' });
		legacy.focus();

		await user.tab();
		expect(close).toHaveFocus();

		await user.tab({ shift: true });
		expect(legacy).toHaveFocus();
	});

	it('shows the labels, because a phone is the one place the rail cannot be read', async () => {
		await sheetAt('/next/dashboard');

		expect(screen.getByText('g d')).toBeInTheDocument();
		expect(screen.getByText('Navigate')).toBeInTheDocument();
	});

	it('closes from its own head control, and never offers to collapse on a phone', async () => {
		const user = userEvent.setup();
		const { store } = await sheetAt('/next/dashboard');
		const close = screen.getByRole('button', { name: 'Close menu' });

		// A phone has no rail to collapse, so the control does not claim a pressed state
		// it cannot change.
		expect(close).not.toHaveAttribute('aria-pressed');

		await user.click(close);

		expect(store.expanded).toBe(false);
	});
});

describe('SideMenu as the column above 768px', () => {
	it('leaves Escape to the page, which is the route that owns it there', async () => {
		const user = userEvent.setup();
		const { store } = menuAt('/next/dashboard');

		await user.keyboard('{Escape}');

		expect(store.expanded).toBe(true);
	});

	it('does not collapse on a route click, so the column survives a navigation', async () => {
		const user = userEvent.setup();
		const { store } = menuAt('/next/dashboard', { stored: { 'paseka:console:side-menu-expanded': 'mini' } });

		await clickRoute(user, 'Traces');

		expect(store.expanded).toBe(false);
	});
});
