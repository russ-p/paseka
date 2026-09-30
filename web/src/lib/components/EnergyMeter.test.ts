import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import EnergyMeter from './EnergyMeter.svelte';

describe('EnergyMeter', () => {
	it('shows the reserve over its denominator and its provenance', () => {
		render(EnergyMeter, {
			energy: { energyBudget: 4, energyRemaining: 2, energyAdded: 6, energyAllocated: 10 },
			ontopup: vi.fn()
		});

		expect(screen.getByText('2 / 10')).toBeInTheDocument();
		expect(screen.getByText('seed 4 · topped 6')).toBeInTheDocument();
	});

	it('badges a low reserve and leaves a healthy one unbadged', () => {
		const { unmount } = render(EnergyMeter, {
			energy: { energyBudget: 12, energyRemaining: 2, lowEnergy: true },
			ontopup: vi.fn()
		});
		const badge = screen.getByText('low');
		expect(badge.className).toContain('badge-warning');
		unmount();

		render(EnergyMeter, { energy: { energyBudget: 12, energyRemaining: 9 }, ontopup: vi.fn() });
		expect(screen.queryByText('low')).not.toBeInTheDocument();
	});

	it('explains an absent reserve instead of showing a bare zero', () => {
		render(EnergyMeter, { energy: {}, ontopup: vi.fn() });

		expect(screen.getByText(/Honey reserve unavailable/)).toBeInTheDocument();
		expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
	});

	it('bounds the bar by the denominator so an over-topped trail cannot overflow it', () => {
		render(EnergyMeter, {
			energy: { energyBudget: 12, energyRemaining: 30, energyAllocated: 12 },
			ontopup: vi.fn()
		});
		const bar = screen.getByRole('progressbar') as HTMLProgressElement;
		expect(bar.max).toBe(12);
		expect(bar.value).toBe(12);
	});

	it('offers the fixed top-up steps and reports the chosen one', async () => {
		const user = userEvent.setup();
		const ontopup = vi.fn();
		render(EnergyMeter, { energy: { energyBudget: 12, energyRemaining: 8 }, ontopup });

		for (const step of ['+1', '+5', '+12']) {
			expect(screen.getByRole('button', { name: step })).toBeInTheDocument();
		}
		await user.click(screen.getByRole('button', { name: '+5' }));
		expect(ontopup).toHaveBeenCalledWith(5);
	});

	it('locks the steps while a top-up is in flight so two cannot overlap', () => {
		render(EnergyMeter, {
			energy: { energyBudget: 12, energyRemaining: 8 },
			pending: true,
			ontopup: vi.fn()
		});
		for (const step of ['+1', '+5', '+12']) {
			expect(screen.getByRole('button', { name: step })).toBeDisabled();
		}
	});

	it('surfaces a failed top-up as an alert', () => {
		render(EnergyMeter, {
			energy: { energyBudget: 12, energyRemaining: 8 },
			error: 'honey reserve not configured',
			ontopup: vi.fn()
		});
		expect(screen.getByRole('alert')).toHaveTextContent('honey reserve not configured');
	});
});
