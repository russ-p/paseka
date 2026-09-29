/**
 * A menu glyph is a *name*, never a component, for the reason `runtimeActionGlyph`
 * returns one: a route table is data, and importing lucide components into it would make
 * the table a module graph. The name is also what a test can assert, and a route whose
 * glyph has no icon in `route-icons.ts` is a type error rather than a blank cell.
 */
export const consoleRoutes = [
	{ path: '/dashboard', label: 'Dashboard', shortcut: 'd', group: 'Work', glyph: 'dashboard' },
	{ path: '/traces', label: 'Traces', shortcut: 't', group: 'Work', glyph: 'route' },
	{ path: '/timeline', label: 'Timeline', shortcut: 'l', group: 'Work', glyph: 'history' },
	{ path: '/tasks', label: 'Tasks', shortcut: 'k', group: 'Work', glyph: 'tasks' },
	{ path: '/reviews', label: 'Reviews', shortcut: 'r', group: 'Work', glyph: 'reviews' },
	{ path: '/sessions', label: 'Sessions', shortcut: 'i', group: 'Work', glyph: 'terminal' },
	{ path: '/bees', label: 'Bees', shortcut: 'b', group: 'Colony', glyph: 'bees' },
	{ path: '/worktrees', label: 'Worktrees', shortcut: 'w', group: 'Colony', glyph: 'worktrees' },
	{ path: '/runs', label: 'Runs', shortcut: 'u', group: 'Colony', glyph: 'runs' },
	{ path: '/git', label: 'Git', shortcut: 'o', group: 'Colony', glyph: 'git' },
	{ path: '/topology', label: 'Topology', shortcut: 'p', group: 'Diagnostics', glyph: 'topology' },
	{ path: '/system', label: 'System', shortcut: 'm', group: 'Diagnostics', glyph: 'system' },
	{ path: '/settings', label: 'Settings', shortcut: 's', group: 'Configuration', glyph: 'settings' }
] as const;

/** Every glyph name the menu can ask for, derived from the table that asks. */
export type RouteGlyph = (typeof consoleRoutes)[number]['glyph'];

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

/**
 * Where the Live bees plaque points. A bee is not addressable, so the plaque
 * routes to the surface its bees are on, in the order the legacy console used
 * and in the order `agentsMeta` lists them: AFK first, then sessions, then
 * Runs. So a colony running both kinds lands on Runs, an interactive-only
 * colony lands on Sessions, and an idle one lands on Runs, where the next bee
 * will appear. Null is the state before the first agents frame arrives.
 */
export function liveBeesPath(
	base: string,
	agents: { afk: number; sessions: number } | null
): string {
	if (agents && agents.afk === 0 && agents.sessions > 0) {
		return consolePath(base, '/sessions');
	}
	return consolePath(base, '/runs');
}

/** The detail surface for one trail; a child path of the Traces menu entry. */
export function traceDetailPath(base: string, traceId: string): string {
	return consolePath(base, `/traces/${encodeURIComponent(traceId)}`);
}

/** One run of one trail; a child path of the Runs menu entry. */
export function runDetailPath(base: string, traceId: string, agentId: string): string {
	return `${consolePath(base, '/runs')}/${encodeURIComponent(traceId)}/${encodeURIComponent(agentId)}`;
}

/** One task of one trail; a child path of the Tasks menu entry. */
export function taskDetailPath(base: string, traceId: string, taskId: string): string {
	return `${consolePath(base, '/tasks')}/${encodeURIComponent(traceId)}/${encodeURIComponent(taskId)}`;
}

/** One proposal awaiting review; a child path of the Reviews menu entry. */
export function reviewDetailPath(base: string, traceId: string, taskId: string): string {
	return `${consolePath(base, '/reviews')}/${encodeURIComponent(traceId)}/${encodeURIComponent(taskId)}`;
}

/** The full-screen merge diff for one proposal, a child of its own detail. */
export function reviewPreviewPath(base: string, traceId: string, taskId: string): string {
	return `${reviewDetailPath(base, traceId, taskId)}/preview`;
}

/** One interactive session; a child path of the Sessions menu entry. */
export function sessionDetailPath(base: string, sessionId: string): string {
	return `${consolePath(base, '/sessions')}/${encodeURIComponent(sessionId)}`;
}

/** The Timeline feed scoped to one trail. */
export function traceTimelinePath(base: string, traceId: string): string {
	return `${consolePath(base, '/timeline')}?trace=${encodeURIComponent(traceId)}`;
}

/** A menu entry is active on its own path and on any child of it. */
export function isRouteActive(href: string, currentPath: string): boolean {
	return currentPath === href || currentPath.startsWith(`${href}/`);
}

/**
 * The list that owns a path, which is what `Escape` returns to — found by reversing
 * `isRouteActive` rather than by a per-route table, so a detail route added later is
 * covered without being registered anywhere. It is the same test the side menu uses to
 * light an entry, run backwards: the entry a trail detail, a run, a task, a proposal, a
 * session, or a merge preview sits under is the list the operator came from, and the
 * extra `/preview` segment is still a child of `/reviews`.
 *
 * `null` when the path is already the list, because there is nowhere for `Escape` to go,
 * and on a route with no detail child for the same reason — `Topology` is not a list of
 * anything an operator would want to be returned to.
 */
export function owningListPath(base: string, currentPath: string): string | null {
	const owner = consoleRoutes.find((route) => isRouteActive(consolePath(base, route.path), currentPath));
	if (!owner) return null;
	const path = consolePath(base, owner.path);
	return path === currentPath ? null : path;
}

/**
 * A dialog owns `Escape` while it is open, and the DOM is where that truth already is:
 * `Modal` renders `role="dialog"` inside `{#if open}` and `Drawer` is that component, so
 * asking the document costs a selector and cannot drift from what is on screen the way a
 * registry of open flags would.
 */
export function dialogOpen(): boolean {
	return document.querySelector('[role="dialog"]') !== null;
}

/**
 * The navigation sheet owns `Escape` while it is open, for the same reason a dialog does:
 * it stands between the operator and the page, and navigating out from under it would
 * leave it open over a different route. It is asked of the DOM rather than of the side
 * menu's state because the two answers can be reached at once and the shell must yield.
 *
 * The `narrow` argument is the whole subtlety. The drawer's checkbox carries one flag for
 * both states — the sheet below 768px, the labels above it — so `checked` alone is true on
 * a desktop showing its labels, and an `Escape` claimed on that reading would strand a
 * trail detail with no way back to its list. Only the narrow state is a sheet; the caller
 * knows which one it is in because the side menu's store reports the breakpoint.
 */
export function navigationOpen(narrow: boolean): boolean {
	if (!narrow) return false;
	return document.querySelector('[data-navigation-toggle]:checked') !== null;
}

/**
 * The control that opens the navigation, so the sheet can hand focus back where it came
 * from. Asked of the DOM for the same reason as `listFilter()`: the element's identity is
 * "the navigation trigger", and the two of them — the trigger in the shell, the panel in
 * the menu — are separate components that must not import each other to agree on a name.
 */
export function navigationTrigger(): HTMLElement | null {
	return document.querySelector<HTMLElement>('[data-navigation-trigger]');
}

/** The first route link in the panel, which is where an opened sheet puts focus. */
export function firstNavigationLink(): HTMLElement | null {
	return document.querySelector<HTMLElement>('[data-navigation-link]');
}

/**
 * The list filter on the current page, if this page has one. A `data-` hook rather than
 * an id, because the element's identity is "the list's filter" and not one particular
 * list's — and because a page that ever grows a second table wants this to find the first
 * without either table having to be named.
 */
export function listFilter(): HTMLInputElement | null {
	return document.querySelector<HTMLInputElement>('[data-list-filter]');
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
