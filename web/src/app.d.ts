declare global {
	namespace App {}
}

/**
 * Vite's `?raw` import surfaces a file as a string. The console shell document is
 * asserted as a file because no component renders it.
 */
declare module '*?raw' {
	const content: string;
	export default content;
}

export {};
