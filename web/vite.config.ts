import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { loadEnv } from 'vite';

const basePath = '/next';
const outputPath = '../internal/console/next/dist';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');

	return {
		plugins: [
			tailwindcss(),
			svelteTesting(),
			sveltekit({
				adapter: adapter({
					pages: outputPath,
					assets: outputPath,
					fallback: '200.html'
				}),
				paths: {
					base: basePath
				}
			})
		],
		server: {
			proxy: {
				'/api': {
					target: env.PASEKA_CONSOLE_API || 'http://127.0.0.1:8787',
					changeOrigin: true,
					ws: true
				},
				// The app mark is served by the Go console from its own static root, and the
				// side menu's head reads it from there. Dev proxies it for the same reason it
				// proxies the API — the asset has one owner, and a dev-only copy of it is the
				// first step towards two.
				'/favicon': {
					target: env.PASEKA_CONSOLE_API || 'http://127.0.0.1:8787',
					changeOrigin: true
				}
			}
		},
		test: {
			expect: { requireAssertions: true },
			environment: 'jsdom',
			setupFiles: ['./src/tests/setup.ts'],
			include: ['src/**/*.{test,spec}.{js,ts}'],
			exclude: ['src/lib/server/**']
		}
	};
});
