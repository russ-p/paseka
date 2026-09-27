import '@testing-library/jest-dom/vitest';
import { afterEach, vi } from 'vitest';

/**
 * A test renderer has no router, so `$app/navigation`'s `replaceState` throws
 * "before router is initialized" — and a table that keeps its filter and page in the URL
 * calls it on every keystroke. The stand-in delegates to the History API, which is the
 * same write the router makes, so a test can assert `window.location.search` and a
 * table can deep-link from one. Everything else in the module is the real export: a
 * suite that needs `goto` recorded still mocks this itself.
 *
 * SvelteKit prints "Avoid using `history.replaceState(...)`" at the History API call.
 * That warning is a false positive here — the thing it warns about conflicting with is
 * the router this environment does not have — and it is left in the output rather than
 * filtered, because a silenced console is how a real one gets missed later.
 */
vi.mock('$app/navigation', async (importOriginal) => ({
	...(await importOriginal<typeof import('$app/navigation')>()),
	replaceState: (url: string | URL, state: App.PageState) =>
		window.history.replaceState(state, '', url)
}));

/**
 * The query belongs to the route in the app — a table cannot outlive its page to leave
 * `?q=` behind for a different route to read — so in a suite it belongs to the test. One
 * test's filter would otherwise seed the next test's table and empty it.
 */
afterEach(() => {
	window.history.replaceState(null, '', '/');
});
