import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Header from './Header.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';

function populatedStore() {
	const store = createConsoleStatusStore();
	store.applyChromeFrame({
		schemaVersion: 1,
		runtime: { status: 'running', alive: true, slug: 'demo' },
		agents: { count: 2, afk: 1, sessions: 1 },
		attention: { reviews: 3, sessions: 1 }
	});
	store.applyDashboard({
		nats: { configured: true, connected: true, ok: true },
		recentTraces: [{ traceId: 'trace-active', hasActive: true }]
	});
	return store;
}

describe('Header', () => {
	it('renders a loading shell before status arrives', () => {
		const { container } = render(Header, { store: createConsoleStatusStore() });

		expect(container.querySelectorAll('.skeleton').length).toBeGreaterThan(0);
		expect(screen.getByRole('status')).toHaveTextContent('Reconnecting to the console event stream.');
	});

	it('renders live stream and dashboard status', () => {
		render(Header, { store: populatedStore() });

		expect(screen.getByText('demo')).toBeInTheDocument();
		expect(screen.getByText('NATS')).toBeInTheDocument();
		expect(screen.getByText('Hive running')).toBeInTheDocument();
		expect(screen.getByText('Bees 2')).toBeInTheDocument();
		expect(screen.getByText('Recent trails 1')).toBeInTheDocument();
		expect(screen.getByText('Reviews 3')).toBeInTheDocument();
	});
});
