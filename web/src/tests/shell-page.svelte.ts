/**
 * A writable stand-in for `$app/state`'s `page.url`.
 *
 * The shell reads the URL for two things `Escape` depends on — which list owns this
 * path, and what to remember as the last route — and both want the same object the
 * router would give. A plain object would let a test pass while the reactivity goes
 * untested, so this is `$state` for the reason `route-params.svelte.ts` is: a route test
 * drives the stand-in rather than remounting, and the reactivity in it is the point.
 */
export const shellPage = $state({ url: new URL('http://localhost/next/traces') });

export function setShellPath(path: string): void {
	shellPage.url = new URL(`http://localhost${path}`);
}
