import { afterEach, describe, expect, it } from 'vitest';
import { readListState, writeListState } from './list-state';

function at(search: string): void {
	window.history.replaceState(null, '', `/next/traces${search}`);
}

describe('listState', () => {
	afterEach(() => {
		window.history.replaceState(null, '', '/');
	});

	it('reads nothing as the first page and an empty filter', () => {
		at('');

		expect(readListState()).toEqual({ q: '', page: 0 });
	});

	it('round-trips what it writes', () => {
		at('');

		writeListState({ q: 'scout', page: 2 });

		expect(window.location.search).toBe('?q=scout&page=2');
		expect(readListState()).toEqual({ q: 'scout', page: 2 });
	});

	it('keeps the filter verbatim, spaces and all, because trimming is the table’s job', () => {
		at('');

		writeListState({ q: '  scout  ', page: 0 });

		// Percent-encoded rather than dropped: a filter that reads back differently from
		// what the operator typed is a filter they cannot correct by editing the link.
		expect(readListState().q).toBe('  scout  ');
	});

	it('spends no query on the default view', () => {
		at('?q=scout&page=2');

		writeListState({ q: '', page: 0 });

		expect(window.location.search).toBe('');
	});

	it('refuses a page the URL cannot be trusted for', () => {
		// `1.5` is the one that matters: a lenient parse rounds it to a page the
		// operator never asked for.
		for (const bad of ['?page=-1', '?page=1.5', '?page=abc', '?page=', '?page=NaN', '?page=+2']) {
			at(bad);
			expect(readListState().page).toBe(0);
		}
	});

	it('leaves the path and the hash alone', () => {
		window.history.replaceState(null, '', '/next/traces/x#events');
		writeListState({ q: 'scout', page: 0 });

		expect(window.location.pathname).toBe('/next/traces/x');
		expect(window.location.hash).toBe('#events');
	});

	it('merges with params it does not own', () => {
		// `trace` is the timeline deep link's param and `q`/`page` are this table's; a
		// keystroke must not take the other one with it on the way out.
		at('?trace=trace-01');

		writeListState({ q: 'scout', page: 1 });

		expect(readListState()).toEqual({ q: 'scout', page: 1 });
		expect(new URLSearchParams(window.location.search).get('trace')).toBe('trace-01');
	});

	it('costs no history write when the query did not change', () => {
		at('?q=scout');
		const before = window.history.length;

		writeListState({ q: 'scout', page: 0 });

		expect(window.history.length).toBe(before);
	});

	it('reads and writes under a key, and leaves the plain params to another table', () => {
		at('?other.q=other&other.page=3&q=ignored');

		expect(readListState('mine')).toEqual({ q: '', page: 0 });
		expect(readListState('other')).toEqual({ q: 'other', page: 3 });

		writeListState({ q: 'mine', page: 2 }, 'mine');

		// Its own key appears, and the unkeyed `q` another table would own is untouched.
		expect(readListState('mine')).toEqual({ q: 'mine', page: 2 });
		expect(readListState('other')).toEqual({ q: 'other', page: 3 });
		expect(new URLSearchParams(window.location.search).get('q')).toBe('ignored');
	});
});
