/**
 * A writable stand-in for `$app/state`'s `page.params`.
 *
 * A `+page.svelte` is a one-liner, and the only thing it can get wrong is the read:
 * a plain `const traceId = page.params.traceId` renders the right proposal on a cold
 * load and then goes stale, because SvelteKit reuses the page component across a
 * sibling link instead of remounting it. Nothing about the body can detect that — it
 * is handed a prop and has no way to know the prop should have moved.
 *
 * So a route test drives this object rather than remounting, and the reactivity in it
 * is the point: a plain object would let a frozen wrapper pass.
 */
export const routeParams = $state<Record<string, string>>({});

export function setRouteParams(next: Record<string, string>): void {
	for (const key of Object.keys(routeParams)) delete routeParams[key];
	Object.assign(routeParams, next);
}
