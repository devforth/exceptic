import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Choice from './Choice.svelte';

describe('Choice', () => {
	it('describes the option with its hint, which a click does not toggle', () => {
		render(Choice, {
			type: 'checkbox',
			label: 'Active: send notifications',
			hint: 'Off: nothing is sent.',
			checked: true,
			onchange: () => {}
		});

		const box = screen.getByRole('checkbox', { name: 'Active: send notifications' });
		const described = document.getElementById(box.getAttribute('aria-describedby') ?? '');

		expect(described?.getAttribute('aria-label')).toBe('Off: nothing is sent.');
		expect(described?.closest('label')).toBeNull();
	});
});
