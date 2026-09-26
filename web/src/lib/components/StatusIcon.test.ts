import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import StatusIcon from './StatusIcon.svelte';

describe('StatusIcon', () => {
	it.each([
		['running', 'running'],
		['stopped', 'stopped'],
		['starting', 'starting'],
		['connected', 'connected'],
		['disconnected', 'disconnected'],
		['failed', 'failed'],
		['whatever', 'whatever']
	])('exposes %s as an accessible name', (status, expected) => {
		render(StatusIcon, { status });

		const icon = screen.getByRole('img', { name: expected });
		expect(icon).toHaveAttribute('title', expected);
		expect(icon).toHaveTextContent(expected);
	});

	it.each([
		['running', 'play'],
		['live', 'play'],
		['stopped', 'stop'],
		['starting', 'pending'],
		['stopping', 'pending'],
		['connected', 'link'],
		['disconnected', 'broken'],
		['failed', 'alert'],
		['killed', 'alert'],
		['idle', 'unknown'],
		['whatever', 'unknown']
	])('draws the %s glyph as %s', (status, glyph) => {
		const { container } = render(StatusIcon, { status });

		expect(container.querySelector(`[data-glyph="${glyph}"]`)).toBeInTheDocument();
	});

	it('renders the overridden glyph, not the status glyph', () => {
		const play = render(StatusIcon, { status: 'stopped', glyph: 'play' });
		expect(play.container.querySelector('[data-glyph="play"]')).toBeInTheDocument();
		expect(play.container.querySelector('[data-glyph="stop"]')).not.toBeInTheDocument();
		expect(play.container.querySelector('path')).toBeInTheDocument();
		expect(play.container.querySelector('rect')).not.toBeInTheDocument();

		const stop = render(StatusIcon, { status: 'running', glyph: 'stop' });
		expect(stop.container.querySelector('[data-glyph="stop"]')).toBeInTheDocument();
		expect(stop.container.querySelector('rect')).toBeInTheDocument();
		expect(stop.container.querySelector('path')).not.toBeInTheDocument();
	});

	it('drops the accessible name when the glyph is decorative inside a control', () => {
		render(StatusIcon, { status: 'stopped', glyph: 'play' });

		expect(screen.queryByRole('img')).not.toBeInTheDocument();
	});

	it('prefers an explicit label over the raw status', () => {
		render(StatusIcon, { status: 'connected', label: 'NATS connected' });

		expect(screen.getByRole('img', { name: 'NATS connected' })).toHaveTextContent('NATS connected');
	});

	it('paints from the semantic tone classes, never raw colors', () => {
		const { container } = render(StatusIcon, { status: 'running' });

		expect(container.querySelector('.text-info')).toBeInTheDocument();
		expect(container.innerHTML).not.toMatch(/#[0-9a-f]{3,6}/i);
		expect(container.innerHTML).not.toMatch(/rgb\(/i);
	});
});
