import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import ThemeSelect from './ThemeSelect.svelte';
import { createThemeStore, themeStorageKey, type ThemeStorage } from '$lib/stores/theme.svelte';

function memoryStorage(): ThemeStorage {
	const values = new Map<string, string>();
	return {
		getItem: (key) => values.get(key) ?? null,
		setItem: (key, value) => values.set(key, value)
	};
}

describe('ThemeSelect', () => {
	it('updates the theme store from the settings form', async () => {
		const user = userEvent.setup();
		const storage = memoryStorage();
		const store = createThemeStore(storage);
		render(ThemeSelect, { store });

		await user.selectOptions(screen.getByRole('combobox'), 'catppuccin-mocha');

		expect(store.current).toBe('catppuccin-mocha');
		expect(storage.getItem(themeStorageKey)).toBe('catppuccin-mocha');
	});
});
