import { describe, expect, it } from 'vitest';
import { createThemeStore, themeStorageKey, type ThemeStorage } from './theme.svelte';

function memoryStorage(initial?: string): ThemeStorage {
	let value = initial ?? null;
	return {
		getItem: () => value,
		setItem: (_key, next) => {
			value = next;
		}
	};
}

describe('themeStore', () => {
	it('persists and applies a curated theme', () => {
		const storage = memoryStorage();
		const applied: string[] = [];
		const store = createThemeStore(storage, (theme) => applied.push(theme));

		store.set('catppuccin-mocha');

		expect(store.current).toBe('catppuccin-mocha');
		expect(storage.getItem(themeStorageKey)).toBe('catppuccin-mocha');
		expect(applied).toEqual(['catppuccin-mocha']);
	});

	it('hydrates from storage and ignores unknown values', () => {
		const storage = memoryStorage('dark');
		const applied: string[] = [];
		const store = createThemeStore(storage, (theme) => applied.push(theme));

		store.hydrate();
		store.set('unknown');

		expect(store.current).toBe('dark');
		expect(store.themes).toHaveLength(6);
		expect(applied).toEqual(['dark']);
	});
});
