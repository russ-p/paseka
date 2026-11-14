import '@testing-library/jest-dom/vitest';
import { afterEach, vi } from 'vitest';
import { installMatchMedia, setViewportNarrow } from './viewport';

installMatchMedia();

/**
 * jsdom implements no `ResizeObserver`, and Svelte's `clientWidth` binding and
 * LayerChart's chart container both construct one. The stub reports nothing,
 * which is the honest answer in an environment with no layout: the chart
 * measures its container as zero wide, so a suite asserts the marks' structure
 * and their theme tokens rather than their pixel geometry.
 */
class ResizeObserverStub implements ResizeObserver {
	observe(): void {}
	unobserve(): void {}
	disconnect(): void {}
}

if (typeof globalThis.ResizeObserver === 'undefined') {
	globalThis.ResizeObserver = ResizeObserverStub;
}

/**
 * jsdom implements no Web Animations API, and Svelte's transitions call
 * `element.animate`. LayerChart animates the mark a click lands on, so a suite
 * that exercises the chart's day selection needs a stand-in: an animation that
 * reports itself finished on the next microtask, because these tests assert the
 * state a click produces, not the motion that led to it.
 */
if (typeof Element.prototype.animate !== 'function') {
	Element.prototype.animate = (() => {
		const animation = {
			cancel(): void {},
			finish(): void {},
			play(): void {},
			pause(): void {},
			reverse(): void {},
			currentTime: 0,
			playState: 'finished' as const,
			onfinish: null as (() => void) | null,
			oncancel: null as (() => void) | null,
			finished: Promise.resolve()
		};
		queueMicrotask(() => animation.onfinish?.());
		return animation;
	}) as unknown as Element['animate'];
}

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
 * test's filter would otherwise seed the next test's table and empty it. The viewport is
 * reset beside it, because a suite that stepped below 768px would otherwise hand the next
 * one a side menu in its sheet state.
 */
afterEach(() => {
	window.history.replaceState(null, '', '/');
	setViewportNarrow(false);
});
