import { describe, expect, it } from 'vitest';
import shell from './app.html?raw';
import appleTouchIcon from '../static/apple-touch-icon.png?raw';
import faviconIco from '../static/favicon.ico?raw';
import faviconSvg from '../static/favicon.svg?raw';

/**
 * The shell document is the one file a route test cannot render: SvelteKit
 * composes it into every built page, so its title and icon set are asserted
 * here rather than inferred from a component. A `/next` link in it would be the
 * cutover's one mechanical failure, so its absence is part of the contract.
 */
describe('console shell document', () => {
	it('names the product rather than the migration', () => {
		expect(shell).toContain('<title>Queen Console</title>');
		expect(shell).not.toContain('Queen Console Next');
	});

	it('declares the bundled favicon and touch-icon set the legacy shell declared', () => {
		expect(shell).toContain('<link rel="icon" href="/favicon.svg" type="image/svg+xml" />');
		expect(shell).toContain('<link rel="icon" href="/favicon.ico" sizes="any" />');
		expect(shell).toContain('<link rel="apple-touch-icon" href="/apple-touch-icon.png" />');
	});

	it('ships those icons from the frontend static root', () => {
		expect(faviconSvg).toContain('<svg');
		expect(faviconIco.length).toBeGreaterThan(0);
		expect(appleTouchIcon.length).toBeGreaterThan(0);
	});

	it('has no retired preview prefix in a link or asset reference', () => {
		expect(shell).not.toMatch(/(?:href|src)="\/next/);
	});
});
