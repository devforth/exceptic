import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import LevelChoices from './LevelChoices.svelte';

describe('LevelChoices', () => {
	it('keeps its legend as the name of the group when it sits beside the levels', async () => {
		const onchange = vi.fn();

		render(LevelChoices, {
			kind: 'log',
			legend: 'Logs',
			selected: ['error', 'fatal'],
			onchange,
			beside: true,
			error: 'Choose a level.'
		});

		const group = screen.getByRole('group', { name: 'Logs' });

		expect(group.getAttribute('aria-describedby')).toBeTruthy();
		expect(screen.getAllByRole('checkbox').map((box) => (box as HTMLInputElement).value)).toEqual([
			'fatal',
			'error',
			'warn',
			'info',
			'debug',
			'trace'
		]);

		await userEvent.click(screen.getByRole('checkbox', { name: 'warn' }));

		expect(onchange).toHaveBeenCalledWith(['fatal', 'error', 'warn']);
	});
});
