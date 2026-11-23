import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Switch from './Switch.svelte';

describe('Switch', () => {
	it('is a switch named by its label, on or off', () => {
		render(Switch, { label: 'Dark mode', checked: true });

		const control = screen.getByRole('switch', { name: 'Dark mode' });
		expect(control.getAttribute('aria-checked')).toBe('true');
	});

	it('toggles on a click and on Space', async () => {
		const user = userEvent.setup();
		const onchange = vi.fn();
		render(Switch, { label: 'Dark mode', checked: false, onchange });

		const control = screen.getByRole('switch', { name: 'Dark mode' });
		await user.click(control);
		expect(onchange).toHaveBeenLastCalledWith(true);
		expect(control.getAttribute('aria-checked')).toBe('true');

		await user.keyboard(' ');
		expect(onchange).toHaveBeenLastCalledWith(false);
		expect(control.getAttribute('aria-checked')).toBe('false');
	});

	it('does nothing when disabled', async () => {
		const user = userEvent.setup();
		const onchange = vi.fn();
		render(Switch, { label: 'Dark mode', checked: false, disabled: true, onchange });

		const control = screen.getByRole('switch', { name: 'Dark mode' });
		await user.click(control);

		expect(onchange).not.toHaveBeenCalled();
		expect(control.getAttribute('aria-checked')).toBe('false');
		expect((control as HTMLButtonElement).disabled).toBe(true);
	});

	it('draws only the track for a row that is the switch itself', () => {
		const { container } = render(Switch, { checked: true, decorative: true });

		expect(screen.queryByRole('switch')).toBeNull();
		expect(container.querySelector('[aria-hidden="true"][data-checked]')).toBeTruthy();
	});
});
