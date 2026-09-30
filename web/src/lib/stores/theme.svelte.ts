export const consoleThemes = [
	'catppuccin-latte',
	'catppuccin-frappe',
	'catppuccin-macchiato',
	'catppuccin-mocha',
	'light',
	'dark'
] as const;

export type ConsoleTheme = (typeof consoleThemes)[number];

export const defaultTheme: ConsoleTheme = 'catppuccin-latte';
export const themeStorageKey = 'paseka:console:theme';

export const consoleThemeLabels: Record<ConsoleTheme, string> = {
	'catppuccin-latte': 'Catppuccin Latte',
	'catppuccin-frappe': 'Catppuccin Frappé',
	'catppuccin-macchiato': 'Catppuccin Macchiato',
	'catppuccin-mocha': 'Catppuccin Mocha',
	light: 'Light',
	dark: 'Dark'
};

export interface ThemeStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

function browserStorage(): ThemeStorage | undefined {
	if (typeof localStorage === 'undefined') return undefined;
	return localStorage;
}

function readTheme(storage: ThemeStorage | undefined): ConsoleTheme {
	try {
		const stored = storage?.getItem(themeStorageKey);
		if (stored && (consoleThemes as readonly string[]).includes(stored)) {
			return stored as ConsoleTheme;
		}
	} catch {
		return defaultTheme;
	}
	return defaultTheme;
}

function writeTheme(storage: ThemeStorage | undefined, theme: ConsoleTheme): void {
	try {
		storage?.setItem(themeStorageKey, theme);
	} catch {
		return;
	}
}

function applyThemeToDocument(theme: ConsoleTheme): void {
	if (typeof document === 'undefined') return;
	document.documentElement.dataset.theme = theme;
}

export function createThemeStore(
	storage: ThemeStorage | undefined = browserStorage(),
	apply: (theme: ConsoleTheme) => void = applyThemeToDocument
) {
	let current = $state<ConsoleTheme>(readTheme(storage));

	return {
		get current(): ConsoleTheme {
			return current;
		},
		get themes(): readonly ConsoleTheme[] {
			return consoleThemes;
		},
		set(theme: string): void {
			if (!(consoleThemes as readonly string[]).includes(theme)) return;
			current = theme as ConsoleTheme;
			writeTheme(storage, current);
			apply(current);
		},
		hydrate(): void {
			current = readTheme(storage);
			apply(current);
		}
	};
}

export type ThemeStore = ReturnType<typeof createThemeStore>;

export const themeStore = createThemeStore();
