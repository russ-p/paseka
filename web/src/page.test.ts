import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import Page from './routes/+page.svelte';

describe('Queen Console Next foundation', () => {
	it('renders the preview without replacing the legacy entry point', () => {
		const { body } = render(Page);

		expect(body).toContain('Queen Console Next');
		expect(body).toContain('href="/"');
	});
});
