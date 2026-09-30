export const sideMenuStorageKey = 'paseka:console:side-menu-expanded';

/**
 * The console's one breakpoint, from the design-system contract (hard rule 7). It is
 * `768px` and not Tailwind's `lg:`, which is `1024px`: the contract says the side menu
 * collapses below `md`, and a drawer that opened at `lg` would show the menu to a
 * `800px` window that was promised a collapsed one.
 */
export const sideMenuDesktopQuery = '(min-width: 768px)';

export interface SideMenuStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

/**
 * Whether the menu is the off-canvas sheet or the persistent column. The drawer's own
 * checkbox cannot answer that, because the same flag means "sheet open" below the
 * breakpoint and "labels shown" above it — the CSS variant would need a media query to
 * read, so the two are asked of JavaScript and the styles follow the flag alone.
 */
export interface SideMenuViewport {
	readonly narrow: boolean;
	watch(onChange: (narrow: boolean) => void): void;
}

function browserStorage(): SideMenuStorage | undefined {
	if (typeof localStorage === 'undefined') return undefined;
	return localStorage;
}

function browserViewport(): SideMenuViewport {
	if (typeof matchMedia === 'undefined') return { narrow: false, watch: () => {} };
	const query = matchMedia(sideMenuDesktopQuery);
	return {
		get narrow() {
			return !query.matches;
		},
		watch(onChange) {
			query.addEventListener('change', (event) => onChange(!event.matches));
		}
	};
}

export function createSideMenuStore(
	storage: SideMenuStorage | undefined = browserStorage(),
	viewport: SideMenuViewport = browserViewport()
) {
	let narrow = $state(viewport.narrow);
	viewport.watch((value) => {
		narrow = value;
	});

	function readStored(): boolean | null {
		try {
			const value = storage?.getItem(sideMenuStorageKey);
			if (value === 'expanded') return true;
			if (value === 'mini') return false;
		} catch {
			return null;
		}
		return null;
	}

	/**
	 * A fresh console shows the labels: user story #3 promises icons *and* labels, and the
	 * rail is the state an operator opts into. A stored `mini` is honoured; a stored
	 * `expanded` is not, on a narrow viewport — there the same flag opens the sheet over
	 * the page, and a preference must not arrive as a panel nobody asked for.
	 */
	function restore(): boolean {
		return (readStored() ?? true) && !narrow;
	}

	let expanded = $state(restore());

	return {
		get expanded(): boolean {
			return expanded;
		},
		get narrow(): boolean {
			return narrow;
		},
		set(value: boolean): void {
			expanded = value;
			// Only a desktop decision is a preference. Closing the sheet on a phone writes
			// nothing, so a phone visit cannot turn the rail off on the operator's laptop.
			if (narrow) return;
			try {
				storage?.setItem(sideMenuStorageKey, value ? 'expanded' : 'mini');
			} catch {
				return;
			}
		},
		/** Re-read the stored preference; the layout calls it on mount beside theme hydration. */
		hydrate(): void {
			expanded = restore();
		}
	};
}

export type SideMenuStore = ReturnType<typeof createSideMenuStore>;

export const sideMenuStore = createSideMenuStore();
