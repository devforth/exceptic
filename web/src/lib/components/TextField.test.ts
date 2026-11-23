import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import TextField from './TextField.svelte';

const descriptionOf = (element: HTMLElement) =>
	(element.getAttribute('aria-describedby') ?? '')
		.split(' ')
		.map((id) => document.getElementById(id)?.textContent?.trim())
		.join(' ');

describe('TextField', () => {
	it('keeps a saved secret in the placeholder and in the description for screen readers', () => {
		render(TextField, { label: 'Bot token', name: 'token', saved: '1234…wxyz' });

		const input = screen.getByLabelText<HTMLInputElement>('Bot token');

		expect(input.placeholder).toBe('saved: 1234…wxyz — leave empty to keep');
		expect(descriptionOf(input)).toBe('Saved: 1234…wxyz. Leave it empty to keep it.');
		expect(input.required).toBe(false);
	});

	it('makes a code field narrow and monospace', () => {
		render(TextField, { label: 'Confirmation code', name: 'code', variant: 'code' });

		const input = screen.getByLabelText('Confirmation code');

		expect(input.className).toContain('font-mono');
		expect(input.className).toContain('tracking-[.15em]');
		expect(input.className).not.toContain('w-full');
	});
});
