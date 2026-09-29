import { describe, expect, it } from 'vitest';
import {
	createSideMenuStore,
	sideMenuStorageKey,
	type SideMenuStorage,
	type SideMenuViewport
} from './side-menu.svelte';

function memoryStorage(initial: Record<string, string> = {}): SideMenuStorage & {
	values: Record<string, string>;
} {
	const values = { ...initial };
	return {
		values,
		getItem: (key: string) => values[key] ?? null,
		setItem: (key: string, value: string) => {
			values[key] = value;
		}
	};
}

function fixedViewport(narrow: boolean): SideMenuViewport & { emit: (narrow: boolean) => void } {
	let onChange: (narrow: boolean) => void = () => {};
	return {
		get narrow() {
			return narrow;
		},
		watch(handler: (narrow: boolean) => void) {
			onChange = handler;
		},
		emit: (value: boolean) => onChange(value)
	};
}

describe('sideMenuStore', () => {
	it('shows the labels on a fresh console, because the rail is the opt-in', () => {
		const store = createSideMenuStore(memoryStorage(), fixedViewport(false));

		expect(store.expanded).toBe(true);
	});

	it('restores a stored rail across a reload', () => {
		const storage = memoryStorage({ [sideMenuStorageKey]: 'mini' });

		expect(createSideMenuStore(storage, fixedViewport(false)).expanded).toBe(false);
		expect(createSideMenuStore(memoryStorage({ [sideMenuStorageKey]: 'expanded' }), fixedViewport(false)).expanded).toBe(true);
	});

	it('does not let a stored preference open the sheet over a phone', () => {
		const storage = memoryStorage({ [sideMenuStorageKey]: 'expanded' });

		expect(createSideMenuStore(storage, fixedViewport(true)).expanded).toBe(false);
	});

	it('writes the two states, and a word rather than a boolean so a value from nowhere is not a choice', () => {
		const storage = memoryStorage();
		const store = createSideMenuStore(storage, fixedViewport(false));

		store.set(false);
		expect(storage.values[sideMenuStorageKey]).toBe('mini');

		store.set(true);
		expect(storage.values[sideMenuStorageKey]).toBe('expanded');
	});

	it('keeps a phone from writing the desktop preference away', () => {
		const storage = memoryStorage();
		const store = createSideMenuStore(storage, fixedViewport(true));

		store.set(false);
		store.set(true);

		expect(storage.values[sideMenuStorageKey]).toBeUndefined();
	});

	it('follows the viewport across the breakpoint', () => {
		const viewport = fixedViewport(false);
		const store = createSideMenuStore(memoryStorage(), viewport);

		expect(store.narrow).toBe(false);

		viewport.emit(true);

		expect(store.narrow).toBe(true);
	});

	it('re-reads the stored preference on hydrate, for the shell that hydrates on mount', () => {
		const storage = memoryStorage();
		const store = createSideMenuStore(storage, fixedViewport(false));
		store.set(false);
		storage.values[sideMenuStorageKey] = 'expanded';

		store.hydrate();

		expect(store.expanded).toBe(true);
	});

	it('treats a word nobody wrote as no preference at all', () => {
		const store = createSideMenuStore(memoryStorage({ [sideMenuStorageKey]: 'wide' }), fixedViewport(false));

		expect(store.expanded).toBe(true);
	});

	it('survives storage that throws, which is what a locked-down browser answers with', () => {
		const store = createSideMenuStore(
			{
				getItem: () => {
					throw new Error('denied');
				},
				setItem: () => {
					throw new Error('denied');
				}
			},
			fixedViewport(false)
		);

		expect(store.expanded).toBe(true);
		expect(() => store.set(false)).not.toThrow();
		expect(store.expanded).toBe(false);
	});

	it('survives having no storage at all, which is a server-rendered pass', () => {
		const store = createSideMenuStore(undefined, fixedViewport(false));

		expect(store.expanded).toBe(true);
		expect(() => store.set(false)).not.toThrow();
		expect(store.expanded).toBe(false);
	});
});
