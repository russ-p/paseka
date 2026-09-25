import { browser } from '$app/environment';
	import { base } from '$app/paths';
	import { landingPath, readStoredRoute } from '$lib/navigation';
	import { redirect } from '@sveltejs/kit';

export function load(): void {
	redirect(307, landingPath(base, browser ? readStoredRoute() : null));
}
