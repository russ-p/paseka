import { describe, expect, it } from 'vitest';
import {
	consolePath,
	consoleRoutes,
	dialogOpen,
	isRouteActive,
	landingPath,
	lastRouteStorageKey,
	listFilter,
	liveBeesPath,
	matchShortcut,
	owningListPath,
	rememberRoute,
	traceDetailPath,
	traceTimelinePath,
	type RouteStorage
} from './navigation';

function memoryStorage(): RouteStorage {
	const values = new Map<string, string>();
	return {
		getItem: (key) => values.get(key) ?? null,
		setItem: (key, value) => {
			values.set(key, value);
		}
	};
}

describe('console navigation', () => {
	it('restores a known last route and rejects unknown paths', () => {
		expect(landingPath('', '/traces')).toBe('/traces');
		expect(landingPath('', '/unknown')).toBe('/dashboard');
		expect(landingPath('', null)).toBe('/dashboard');
	});

	it('ignores a last route left under the retired preview prefix', () => {
		// A tab that stored its path before the cutover must land on the Dashboard
		// rather than on a path the root route table no longer declares.
		expect(landingPath('', '/next/traces')).toBe('/dashboard');
		expect(landingPath('', '/next/traces/trace-01a0bd6963faa14f')).toBe('/dashboard');
		expect(landingPath('', '/next')).toBe('/dashboard');
	});

	it('persists only declared console routes', () => {
		const storage = memoryStorage();

		rememberRoute('', consolePath('', '/worktrees'), storage);
		rememberRoute('', '/outside', storage);
		rememberRoute('', '/next/traces', storage);

		expect(storage.getItem(lastRouteStorageKey)).toBe('/worktrees');
	});

	it('resolves g-chords across the route set', () => {
		expect(matchShortcut(false, 'g')).toEqual({ pending: true });
		expect(matchShortcut(true, 't')).toEqual({ pending: false, path: '/traces' });
		expect(matchShortcut(true, 'x')).toEqual({ pending: false, path: undefined });
	});

	it('keeps shortcuts unique and menu groups contiguous', () => {
		const shortcuts = consoleRoutes.map((route) => route.shortcut);
		const groups = consoleRoutes.map((route) => route.group);
		const changes = groups.filter((group, index) => index > 0 && group !== groups[index - 1]);

		expect(new Set(shortcuts).size).toBe(shortcuts.length);
		expect(changes).toHaveLength(new Set(groups).size - 1);
	});
});

describe('trace paths', () => {
	it('builds the detail route at the console root and escapes the id', () => {
		expect(traceDetailPath('', 'trace-01a0')).toBe('/traces/trace-01a0');
		expect(traceDetailPath('', 'trace/../evil')).toBe('/traces/trace%2F..%2Fevil');
	});

	it('scopes the timeline feed to one trail', () => {
		expect(traceTimelinePath('', 'trace-01a0')).toBe('/timeline?trace=trace-01a0');
		expect(traceTimelinePath('', 'trace/../evil')).toBe(
			'/timeline?trace=trace%2F..%2Fevil'
		);
	});
});

describe('isRouteActive', () => {
	it('marks a menu entry active on its own path and on any child of it', () => {
		expect(isRouteActive('/traces', '/traces')).toBe(true);
		expect(isRouteActive('/traces', '/traces/trace-01a0')).toBe(true);
		expect(isRouteActive('/traces', '/tracesomething')).toBe(false);
		expect(isRouteActive('/traces', '/timeline')).toBe(false);
	});
});

describe('owningListPath', () => {
	/**
	 * The five detail families, plus the merge preview that is a grandchild. Each is
	 * covered by the same reversed `isRouteActive`, so a detail route added later needs
	 * no registration here or anywhere else.
	 */
	const details = [
		'/traces/trace-01a0bd6963faa14f',
		'/runs/trace-01a0bd6963faa14f/run-01',
		'/tasks/trace-01a0bd6963faa14f/task-b2',
		'/reviews/trace-01a0bd6963faa14f/task-b2',
		'/sessions/session-01',
		'/reviews/trace-01a0bd6963faa14f/task-b2/preview'
	];

	it('returns every detail route to the list the operator came from', () => {
		expect(owningListPath('', details[0])).toBe('/traces');
		expect(owningListPath('', details[1])).toBe('/runs');
		expect(owningListPath('', details[2])).toBe('/tasks');
		expect(owningListPath('', details[3])).toBe('/reviews');
		expect(owningListPath('', details[4])).toBe('/sessions');
		// The preview is a child of a child, and the extra segment must not hide the owner.
		expect(owningListPath('', details[5])).toBe('/reviews');
	});

	it('has nowhere to go from a list, because a list is not inside another list', () => {
		expect(owningListPath('', '/traces')).toBeNull();
		expect(owningListPath('', '/dashboard')).toBeNull();
	});

	it('has nowhere to go from a route that is not a list of anything', () => {
		// Topology is one graph, and Settings one form: neither is inside a list an
		// operator would be escaping from.
		expect(owningListPath('', '/topology')).toBeNull();
		expect(owningListPath('', '/settings')).toBeNull();
	});

	it('has nowhere to go from a path it does not know, rather than guessing a parent', () => {
		expect(owningListPath('', '/nope')).toBeNull();
		expect(owningListPath('', '/')).toBeNull();
	});
});

describe('shell DOM probes', () => {
	it('reports a dialog only while one is on screen', () => {
		expect(dialogOpen()).toBe(false);
		const dialog = document.createElement('div');
		dialog.setAttribute('role', 'dialog');
		document.body.append(dialog);

		expect(dialogOpen()).toBe(true);

		dialog.remove();
		expect(dialogOpen()).toBe(false);
	});

	it('finds the list filter by what it is, not by which list it belongs to', () => {
		expect(listFilter()).toBeNull();
		const input = document.createElement('input');
		input.setAttribute('data-list-filter', '');
		document.body.append(input);

		expect(listFilter()).toBe(input);

		input.remove();
		expect(listFilter()).toBeNull();
	});
});

describe('liveBeesPath', () => {
	it('sends a mixed colony to Runs, because AFK is checked first', () => {
		expect(liveBeesPath('', { afk: 1, sessions: 1 })).toBe('/runs');
		expect(liveBeesPath('', { afk: 1, sessions: 0 })).toBe('/runs');
	});

	it('sends sessions-only bees to Sessions and an idle plaque to Runs', () => {
		expect(liveBeesPath('', { afk: 0, sessions: 2 })).toBe('/sessions');
		expect(liveBeesPath('', { afk: 0, sessions: 0 })).toBe('/runs');
	});

	it('falls back to Runs before the first agents frame arrives', () => {
		expect(liveBeesPath('', null)).toBe('/runs');
	});
});
