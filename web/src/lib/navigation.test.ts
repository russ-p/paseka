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
		expect(landingPath('/next', '/next/traces')).toBe('/next/traces');
		expect(landingPath('/next', '/next/unknown')).toBe('/next/dashboard');
		expect(landingPath('/next', null)).toBe('/next/dashboard');
	});

	it('persists only declared console routes', () => {
		const storage = memoryStorage();

		rememberRoute('/next', consolePath('/next', '/worktrees'), storage);
		rememberRoute('/next', '/outside', storage);

		expect(storage.getItem(lastRouteStorageKey)).toBe('/next/worktrees');
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
	it('builds the detail route under the base and escapes the id', () => {
		expect(traceDetailPath('/next', 'trace-01a0')).toBe('/next/traces/trace-01a0');
		expect(traceDetailPath('/next', 'trace/../evil')).toBe('/next/traces/trace%2F..%2Fevil');
	});

	it('scopes the timeline feed to one trail', () => {
		expect(traceTimelinePath('/next', 'trace-01a0')).toBe('/next/timeline?trace=trace-01a0');
		expect(traceTimelinePath('/next', 'trace/../evil')).toBe(
			'/next/timeline?trace=trace%2F..%2Fevil'
		);
	});
});

describe('isRouteActive', () => {
	it('marks a menu entry active on its own path and on any child of it', () => {
		expect(isRouteActive('/next/traces', '/next/traces')).toBe(true);
		expect(isRouteActive('/next/traces', '/next/traces/trace-01a0')).toBe(true);
		expect(isRouteActive('/next/traces', '/next/tracesomething')).toBe(false);
		expect(isRouteActive('/next/traces', '/next/timeline')).toBe(false);
	});
});

describe('owningListPath', () => {
	/**
	 * The five detail families, plus the merge preview that is a grandchild. Each is
	 * covered by the same reversed `isRouteActive`, so a detail route added later needs
	 * no registration here or anywhere else.
	 */
	const details = [
		'/next/traces/trace-01a0bd6963faa14f',
		'/next/runs/trace-01a0bd6963faa14f/run-01',
		'/next/tasks/trace-01a0bd6963faa14f/task-b2',
		'/next/reviews/trace-01a0bd6963faa14f/task-b2',
		'/next/sessions/session-01',
		'/next/reviews/trace-01a0bd6963faa14f/task-b2/preview'
	];

	it('returns every detail route to the list the operator came from', () => {
		expect(owningListPath('/next', details[0])).toBe('/next/traces');
		expect(owningListPath('/next', details[1])).toBe('/next/runs');
		expect(owningListPath('/next', details[2])).toBe('/next/tasks');
		expect(owningListPath('/next', details[3])).toBe('/next/reviews');
		expect(owningListPath('/next', details[4])).toBe('/next/sessions');
		// The preview is a child of a child, and the extra segment must not hide the owner.
		expect(owningListPath('/next', details[5])).toBe('/next/reviews');
	});

	it('has nowhere to go from a list, because a list is not inside another list', () => {
		expect(owningListPath('/next', '/next/traces')).toBeNull();
		expect(owningListPath('/next', '/next/dashboard')).toBeNull();
	});

	it('has nowhere to go from a route that is not a list of anything', () => {
		// Topology is one graph, and Settings one form: neither is inside a list an
		// operator would be escaping from.
		expect(owningListPath('/next', '/next/topology')).toBeNull();
		expect(owningListPath('/next', '/next/settings')).toBeNull();
	});

	it('has nowhere to go from a path it does not know, rather than guessing a parent', () => {
		expect(owningListPath('/next', '/next/nope')).toBeNull();
		expect(owningListPath('/next', '/')).toBeNull();
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
		expect(liveBeesPath('/next', { afk: 1, sessions: 1 })).toBe('/next/runs');
		expect(liveBeesPath('/next', { afk: 1, sessions: 0 })).toBe('/next/runs');
	});

	it('sends sessions-only bees to Sessions and an idle plaque to Runs', () => {
		expect(liveBeesPath('/next', { afk: 0, sessions: 2 })).toBe('/next/sessions');
		expect(liveBeesPath('/next', { afk: 0, sessions: 0 })).toBe('/next/runs');
	});

	it('falls back to Runs before the first agents frame arrives', () => {
		expect(liveBeesPath('/next', null)).toBe('/next/runs');
	});
});
