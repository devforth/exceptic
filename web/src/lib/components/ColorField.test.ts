import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { projectPalette } from '#lib/api/catalog.gen.ts';
import ColorField from './ColorField.svelte';

vi.mock('$app/state', () => ({ page: { data: {} } }));

describe('ColorField', () => {
	it('offers the palette as named swatches, the current one checked', () => {
		render(ColorField, { value: projectPalette[1].color, name: 'mobile' });

		for (const entry of projectPalette) {
			expect(screen.getByRole('radio', { name: entry.name })).toBeTruthy();
		}

		expect(
			screen.getByRole<HTMLInputElement>('radio', { name: projectPalette[1].name }).checked
		).toBe(true);
	});

	it('previews the chosen swatch on the badge', async () => {
		render(ColorField, { value: projectPalette[0].color, name: 'mobile' });

		await userEvent.click(screen.getByRole('radio', { name: projectPalette[2].name }));

		const badge = screen.getByText('mobile');

		expect(badge.getAttribute('data-project')).toBe('mobile');
		expect(badge.style.getPropertyValue('--project-color')).toBe(projectPalette[2].color);
	});

	it('checks no swatch for a color of its own', () => {
		render(ColorField, { value: '#123456', name: 'mobile', error: 'Choose a color.' });

		expect(screen.queryAllByRole('radio', { checked: true })).toHaveLength(0);
		expect(screen.getByLabelText<HTMLInputElement>('Custom color').value).toBe('#123456');
		expect(screen.getByText('Choose a color.')).toBeTruthy();
	});
});
