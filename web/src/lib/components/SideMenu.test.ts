import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { tick } from 'svelte';
import { describe, expect, it } from 'vitest';
import SideMenu from './SideMenu.svelte';
import { consoleRoutes } from '$lib/navigation';
import { createSideMenuStore, type SideMenuStorage, type SideMenuViewport } from '$lib/stores/side-menu.svelte';
import { createVersionStore } from '$lib/stores/version.svelte';
import { buildView, buildViewDev } from '../../tests/fixtures';
import type { BuildView } from '$lib/api/types';

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
	options: {
		stored?: Record<string, string>;
		narrow?: boolean;
		build?: BuildView | null;
	} = {}
) {
	const storage = memoryStorage(options.stored ?? {});
	const viewport = fixedViewport(options.narrow ?? false);
	const store = createSideMenuStore(storage, viewport);
	// `null` means the stamp never arrived, which is what a console that has not
	// finished its first read looks like — the default rather than a real release.
	const version = createVersionStore({
		loadVersion: async () => {
			if (options.build === null) throw new Error('connection refused');
			return options.build ?? buildView();
		}
	});
	const view = render(SideMenu, { store, currentPath, version });
	return { storage, viewport, store, version, ...view };
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
describe('SideMenu build stamp', () => {
	/** The layout owns the read, so a test asks for it the way the layout does. */
	async function stampedAt(
		currentPath: string,
		options: {
			build?: BuildView | null;
			stored?: Record<string, string>;
			narrow?: boolean;
		} = {}
	) {
		const made = menuAt(currentPath, options);
		await made.version.refresh();
		return made;
	}

	it('names the build in the foot, because that is the one fact a report needs', async () => {
		await stampedAt('/next/dashboard');

		const stamp = screen.getByText('0.5.0+67730c4');
		expect(stamp).toBeInTheDocument();
		// The tooltip carries what the label cannot: the sha to paste, and whether
		// this build is one anybody else is running.
		expect(stamp.getAttribute('title')).toContain('67730c4da70fbd912a575fe614e5e248c402fdf0');
	});

	it('shows the commit alone in the rail, which is all 16 characters hold', async () => {
		const { store } = await stampedAt('/next/dashboard');

		store.set(false);
		await waitFor(() => expect(screen.queryByText('0.5.0+67730c4')).toBeNull());

		expect(screen.getByText('67730c4')).toBeInTheDocument();
	});

	it('shows a development build as itself, dirty state included', async () => {
		// `dev` with a commit is a build from main, not a broken install, and an
		// operator has to be able to tell those two apart at a glance.
		await stampedAt('/next/dashboard', { build: buildViewDev({ dirty: true, display: 'dev+67730c4.dirty' }) });

		const stamp = screen.getByText('dev+67730c4.dirty');
		expect(stamp.getAttribute('title')).toContain('not a tagged release');
		expect(stamp.getAttribute('title')).toContain('dirty tree');
	});

	it('draws nothing at all when the stamp never arrives', async () => {
		await stampedAt('/next/dashboard', { build: null });

		// Silence rather than a dash: the footer is where an operator confirms which
		// build they are reading, and a placeholder there reads as an answer.
		expect(document.querySelector('#side-menu-version')).toBeNull();
	});

	it('makes the version itself the link, because a sha with nowhere to go is half an answer', async () => {
		await stampedAt('/next/dashboard');

		// One link, on the version, rather than a button beside it: the route list is
		// the one part of the panel that scrolls, and a row the foot claims is a route
		// it hides.
		const stamp = screen.getByRole('link', { name: '0.5.0+67730c4 — open on GitHub' });
		expect(stamp).toHaveAttribute(
			'href',
			'https://github.com/russ-p/paseka/commit/67730c4da70fbd912a575fe614e5e248c402fdf0'
		);
		// `noopener noreferrer` on an external link: the opened tab gets no handle on
		// this one.
		expect(stamp).toHaveAttribute('target', '_blank');
		expect(stamp).toHaveAttribute('rel', 'noopener noreferrer');
		expect(document.querySelector('#side-menu-repository')).toBeNull();
	});

	it('falls back to the repository when the build has no commit to show', async () => {
		// An unstamped binary names no commit, so there is no commit page to open; the
		// repository is still the better answer than a link nowhere.
		await stampedAt('/next/dashboard', {
			build: buildViewDev({ commit: undefined, shortCommit: undefined, commitUrl: undefined, display: 'dev' })
		});

		expect(screen.getByRole('link', { name: 'dev — open on GitHub' })).toHaveAttribute(
			'href',
			'https://github.com/russ-p/paseka'
		);
	});

	it('keeps the rail version clickable under a name that still carries it', async () => {
		const { store } = await stampedAt('/next/dashboard');

		store.set(false);
		await waitFor(() => expect(screen.queryByText('0.5.0+67730c4')).toBeNull());

		// The accessible name carries the visible text in both states, so the rail's
		// bare sha is never the only thing naming the link.
		expect(screen.getByRole('link', { name: '67730c4 — open on GitHub' })).toBeInTheDocument();
	});

	it('draws no link at all when the console could not read its own build', async () => {
		await stampedAt('/next/dashboard', { build: null });

		// A link built from a guess would send an operator into a repository this
		// console was not built from, so an absent stamp leaves the footer empty.
		expect(document.querySelector('#side-menu-version')).toBeNull();
		expect(screen.queryByRole('link', { name: /GitHub/ })).not.toBeInTheDocument();
	});

	it('keeps the legacy console link last in the sheet, so the trap wraps on real ends', async () => {
		const user = userEvent.setup();
		const { store } = await stampedAt('/next/dashboard', { narrow: true });
		store.set(true);
		await tick();

		const close = screen.getByRole('button', { name: 'Close menu' });
		const version = screen.getByRole('link', { name: /open on GitHub/ });
		const legacy = screen.getByRole('link', { name: 'Open legacy console' });
		legacy.focus();

		await user.tab();
		expect(close).toHaveFocus();

		await user.tab({ shift: true });
		expect(legacy).toHaveFocus();

		// The version link sits between the two ends of the trap, not outside them.
		version.focus();
		await user.tab();
		expect(legacy).toHaveFocus();
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
