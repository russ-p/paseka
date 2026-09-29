const desktop = '(min-width: 768px)';

const listeners = new Set<(matches: boolean) => void>();
let narrow = false;

/**
 * jsdom has no layout, so `matchMedia` cannot answer the console's one breakpoint. The
 * stub reports a desktop viewport — the width a console is actually read at — and lets a
 * suite step across it, so "the side menu is the sheet" is something a test asks for
 * rather than something it inherits from the environment.
 */
export function installMatchMedia(): void {
	window.matchMedia = ((query: string) => ({
		get matches() {
			return query === desktop ? !narrow : false;
		},
		media: query,
		onchange: null,
		addEventListener: (_type: string, handler: (event: { matches: boolean }) => void) => {
			listeners.add((matches) => handler({ matches } as MediaQueryListEvent));
		},
		removeEventListener: () => {},
		addListener: () => {},
		removeListener: () => {},
		dispatchEvent: () => false
	})) as unknown as typeof window.matchMedia;
}

export function setViewportNarrow(value: boolean): void {
	narrow = value;
	for (const listener of listeners) listener(!narrow);
}
