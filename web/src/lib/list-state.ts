/**
 * A table's filter and page, as the URL carries them.
 *
 * User story #3 promises switching contexts "without losing scroll position", and
 * `?q=&page=` is the half of that promise a list can keep by itself: Back from a trail
 * detail returns the operator to the page of the list they left, and a filtered list is
 * a link they can send somebody. The other half — restoring the scroll offset of a long
 * page that has no page number — is per surface and has no URL to live in.
 *
 * `replaceState`, never `pushState`: a filter is not navigation, and Back must not walk
 * an operator backwards through the letters of a word. It also means the table never has
 * to react to a history move, because nothing it does ever creates a history entry to
 * move between.
 */
import { untrack } from 'svelte';
import { replaceState } from '$app/navigation';
import { page } from '$app/state';

export interface ListState {
	/** The filter text verbatim — trimming is the table's business, not the URL's. */
	q: string;
	/** Zero-based page index, so the default costs no query at all. */
	page: number;
}

/**
 * `stateKey` namespaces both params for a route that carries more than one table.
 * Every route has exactly one today, so it is absent and the query stays the readable
 * `?q=&page=`; a second table sets it rather than this module inventing a key out of a
 * label written for a human, which would put `?Review queue.q=` on the wire.
 */
function paramNames(stateKey?: string): { q: string; page: string } {
	return stateKey
		? { q: `${stateKey}.q`, page: `${stateKey}.page` }
		: { q: 'q', page: 'page' };
}

/**
 * A hand-edited `?page=` is untrusted, and it is read strictly rather than leniently:
 * `parseInt('1.5')` is `1`, and a page index that arrived as a fraction is a number
 * somebody got wrong, not a page to round in their favour.
 */
function readPage(raw: string | null): number {
	if (raw === null || !/^\d+$/.test(raw)) return 0;
	return Number.parseInt(raw, 10);
}

/**
 * The table's seed, read once when it mounts. Untracked, and read from the address bar
 * rather than from `$app/state`: the router's copy is reactive, and a table that
 * re-seeded on every URL write would feed its own write-back straight back into itself.
 * Nothing else ever changes the query behind a mounted table, so there is no second
 * read to miss.
 */
export function readListState(stateKey?: string): ListState {
	const names = paramNames(stateKey);
	const search = new URLSearchParams(window.location.search);
	return { q: search.get(names.q) ?? '', page: readPage(search.get(names.page)) };
}

/**
 * Publish the view. The page written is the one on screen, not the one requested: a
 * bookmarked `page=9` that a poll has invalidated is clamped for display, and leaving
 * the stale number in the address bar would make the URL disagree with the table under
 * it — so this also corrects the URL, once, on the way past.
 */
export function writeListState(state: ListState, stateKey?: string): void {
	const names = paramNames(stateKey);
	const url = new URL(window.location.href);
	if (state.q === '') url.searchParams.delete(names.q);
	else url.searchParams.set(names.q, state.q);
	if (state.page <= 0) url.searchParams.delete(names.page);
	else url.searchParams.set(names.page, String(state.page));

	// Unrelated params are somebody else's — `?trace=` on the timeline's deep link — so
	// this merges rather than replaces, and a keystroke that does not change the query
	// costs no history write.
	const query = url.search;
	if (query === window.location.search) return;
	// The hash rides along: an anchored note or a folded section is a position the
	// operator can be sent to, and dropping it to publish a filter would move them.
	replaceState(`${url.pathname}${query}${url.hash}`, untrack(() => page.state));
}
