import { describe, expect, it } from 'vitest';
import {
	consolePath,
	consoleRoutes,
	isRouteActive,
	landingPath,
	lastRouteStorageKey,
	matchShortcut,
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
