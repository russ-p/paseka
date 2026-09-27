import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createToastStore, toastClass, type ToastTone } from './toast.svelte';

describe('toastClass', () => {
	it('maps each tone to the alert class the design system owns', () => {
		const expected: Record<ToastTone, string> = {
			info: 'alert-info',
			success: 'alert-success',
			warning: 'alert-warning',
			error: 'alert-error'
		};
		for (const [tone, className] of Object.entries(expected)) {
			expect(toastClass(tone as ToastTone)).toBe(className);
		}
	});

	it('gives every tone its own class, because a report that shares one reads as another', () => {
		// A page cannot invent a palette beside the rest of the console, and the way to
		// guarantee that is that no two tones render identically.
		const classes = (['info', 'success', 'warning', 'error'] as ToastTone[]).map(toastClass);
		expect(new Set(classes).size).toBe(classes.length);
	});
});

describe('toastStore', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('starts empty, so a page that reports nothing shows no region content', () => {
		const store = createToastStore();

		expect(store.items).toEqual([]);
	});

	it('holds a report in the order it arrived', () => {
		const store = createToastStore(0);

		store.push('info', 'Committed to paseka/trace-1');
		store.push('error', 'origin unreachable');

		expect(store.items.map((item) => item.message)).toEqual([
			'Committed to paseka/trace-1',
			'origin unreachable'
		]);
	});

	it('gives every report its own id, which is what the dismiss button and the timer act on', () => {
		const store = createToastStore(0);

		const first = store.push('info', 'first');
		const second = store.push('info', 'second');

		// Two reports with one id would leave the first un-dismissable, and a keyless
		// list would make two identical messages one entry.
		expect(first).not.toBe(second);
		expect(store.items.map((item) => item.id)).toEqual([first, second]);
	});

	it('keeps the tone with the message, because the tone is the whole report on a busy page', () => {
		const store = createToastStore(0);

		store.push('warning', 'Deleted 2 of 3; one branch was refused.');

		expect(store.items[0]).toMatchObject({
			tone: 'warning',
			message: 'Deleted 2 of 3; one branch was refused.'
		});
	});

	it('dismisses one report without touching the rest', () => {
		const store = createToastStore(0);
		const first = store.push('info', 'first');
		store.push('info', 'second');

		store.dismiss(first);

		expect(store.items.map((item) => item.message)).toEqual(['second']);
	});

	it('ignores a dismissal for a report that is already gone', () => {
		const store = createToastStore(0);
		const first = store.push('info', 'first');
		store.dismiss(first);

		// A timer and a click can both land on the same report, and the second must be
		// a no-op rather than an error on a page that is only reporting.
		expect(() => store.dismiss(first)).not.toThrow();
		expect(store.items).toEqual([]);
	});

	it('goes away on its own after the timeout', () => {
		const store = createToastStore(4000);
		store.push('success', 'Merged into main.');

		vi.advanceTimersByTime(3999);
		expect(store.items).toHaveLength(1);

		vi.advanceTimersByTime(1);
		expect(store.items).toEqual([]);
	});

	it('times each report from its own arrival, so a burst drains rather than emptying at once', () => {
		const store = createToastStore(4000);
		store.push('success', 'Merged into main.');

		vi.advanceTimersByTime(3000);
		store.push('success', 'Committed to paseka/trace-1');
		expect(store.items).toHaveLength(2);

		// A report that arrived a second after a push is still there when the first
		// one's timer fires, so the second failure is not swallowed by the first.
		vi.advanceTimersByTime(1000);
		expect(store.items.map((item) => item.message)).toEqual(['Committed to paseka/trace-1']);

		vi.advanceTimersByTime(4000);
		expect(store.items).toEqual([]);
	});

	it('keeps a report for good when the timeout is off', () => {
		const store = createToastStore(0);
		store.push('info', 'Fetch complete');

		vi.advanceTimersByTime(600_000);

		// `0` is how a page with its own reporting opts out, and a store that ignored it
		// would empty the region a moment after every report.
		expect(store.items).toHaveLength(1);
	});

	it('does not arm a timer for a report nobody will dismiss by hand', () => {
		const store = createToastStore(0);

		store.push('info', 'Fetch complete');
		expect(vi.getTimerCount()).toBe(0);
	});

	it('replaces the list rather than mutating it, so a reader sees the new report', () => {
		const store = createToastStore(0);
		const before = store.items;

		store.push('info', 'first');

		// `$state` tracks by reference, so an in-place push would not invalidate
		// anything and the toast region would keep showing nothing.
		expect(store.items).not.toBe(before);
	});

	it('arms one timer per report, and none for a dismissal that beat it', () => {
		const store = createToastStore(4000);
		const first = store.push('info', 'first');
		store.push('info', 'second');
		expect(vi.getTimerCount()).toBe(2);

		store.dismiss(first);

		// The timer is left to fire and find nothing, which is cheaper than tracking
		// which ones are still live.
		vi.advanceTimersByTime(4000);
		expect(store.items).toEqual([]);
	});
});
