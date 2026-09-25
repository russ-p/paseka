export const consoleRoutes = [
	{ path: '/dashboard', label: 'Dashboard', shortcut: 'd', group: 'Work' },
	{ path: '/traces', label: 'Traces', shortcut: 't', group: 'Work' },
	{ path: '/timeline', label: 'Timeline', shortcut: 'l', group: 'Work' },
	{ path: '/tasks', label: 'Tasks', shortcut: 'k', group: 'Work' },
	{ path: '/reviews', label: 'Reviews', shortcut: 'r', group: 'Work' },
	{ path: '/sessions', label: 'Sessions', shortcut: 'i', group: 'Work' },
	{ path: '/bees', label: 'Bees', shortcut: 'b', group: 'Colony' },
	{ path: '/worktrees', label: 'Worktrees', shortcut: 'w', group: 'Colony' },
	{ path: '/runs', label: 'Runs', shortcut: 'u', group: 'Colony' },
	{ path: '/git', label: 'Git', shortcut: 'o', group: 'Colony' },
	{ path: '/topology', label: 'Topology', shortcut: 'p', group: 'Diagnostics' },
	{ path: '/system', label: 'System', shortcut: 'm', group: 'Diagnostics' },
	{ path: '/settings', label: 'Settings', shortcut: 's', group: 'Configuration' }
] as const;

export const lastRouteStorageKey = 'paseka:console:last-route';

export interface RouteStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

function browserStorage(): RouteStorage | undefined {
	if (typeof localStorage === 'undefined') return undefined;
	return localStorage;
}

export function consolePath(base: string, path: string): string {
	return `${base}${path}`;
}

export function readStoredRoute(storage: RouteStorage | undefined = browserStorage()): string | null {
	try {
		return storage?.getItem(lastRouteStorageKey) ?? null;
	} catch {
		return null;
	}
}

export function rememberRoute(
	base: string,
	currentPath: string,
	storage: RouteStorage | undefined = browserStorage()
): void {
	const known = consoleRoutes.some((route) => consolePath(base, route.path) === currentPath);
	if (!known || !storage) return;
	try {
		storage.setItem(lastRouteStorageKey, currentPath);
	} catch {
		return;
	}
}

export function landingPath(base: string, storedPath: string | null): string {
	const stored = consoleRoutes.some((route) => consolePath(base, route.path) === storedPath)
		? storedPath
		: null;
	return stored ?? consolePath(base, consoleRoutes[0].path);
}

export function routeForShortcut(key: string): string | undefined {
	return consoleRoutes.find((route) => route.shortcut === key.toLowerCase())?.path;
}

export function matchShortcut(
	pending: boolean,
	key: string
): { pending: boolean; path?: string } {
	if (pending) return { pending: false, path: routeForShortcut(key) };
	if (key.toLowerCase() === 'g') return { pending: true };
	return { pending: false };
}
