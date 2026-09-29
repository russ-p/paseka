import {
	FolderGit2,
	GitBranch,
	Hexagon,
	History,
	LayoutDashboard,
	ListTodo,
	Route,
	Server,
	Settings,
	Share2,
	SquareCheckBig,
	Terminal,
	Zap
} from 'lucide-svelte';
import type { RouteGlyph } from '$lib/navigation';

/**
 * The navigation glyphs, keyed by the names `consoleRoutes` carries. This is the same
 * split `StatusIcon` and `runtimeActionGlyph` use: the table stays data, and one map here
 * turns a name into an icon — so a route asking for a glyph with no icon is a type error
 * here rather than a blank cell in the menu, and the keys cannot drift from the table
 * because `RouteGlyph` is derived from it.
 *
 * `Hexagon` is the Bees roster's glyph because lucide has no `bee` (only `beef`): a comb
 * cell is also a truer picture of what the page lists than a cow is. `Zap` is a headless
 * run against `Terminal` for a session, since the two run things and only one has a
 * person in it.
 */
export const routeGlyphs: Record<RouteGlyph, typeof LayoutDashboard> = {
	dashboard: LayoutDashboard,
	route: Route,
	history: History,
	tasks: ListTodo,
	reviews: SquareCheckBig,
	terminal: Terminal,
	bees: Hexagon,
	worktrees: FolderGit2,
	runs: Zap,
	git: GitBranch,
	topology: Share2,
	system: Server,
	settings: Settings
};
