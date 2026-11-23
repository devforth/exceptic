import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Select from './Select.svelte';

const options = [
	{ value: 'unresolved', label: 'Unresolved' },
	{ value: 'resolved', label: 'Resolved' },
	{ value: 'ignored', label: 'Ignored' }
];

function renderSelect(value = 'resolved', all?: string) {
	const onchange = vi.fn();

	render(Select, { label: 'Status', value, options, onchange, all });

	return { onchange, button: screen.getByRole('button', { name: /^Status/ }) };
}

const listbox = () => screen.queryByRole('listbox', { name: 'Status' });

describe('Select', () => {
	it('names the choice on its button and checks it in the list', async () => {
		const { button } = renderSelect();

		expect(button.getAttribute('aria-haspopup')).toBe('listbox');
		expect(button.getAttribute('aria-expanded')).toBe('false');
		expect(button.textContent).toContain('Resolved');

		await userEvent.click(button);

		expect(button.getAttribute('aria-expanded')).toBe('true');
		expect(
			screen.getAllByRole('option').map((option) => option.getAttribute('aria-selected'))
		).toEqual(['false', 'true', 'false']);
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Resolved' }));
	});

	it('reports a click on another option and closes back on the button', async () => {
		const { onchange, button } = renderSelect();

		await userEvent.click(button);
		await userEvent.click(screen.getByRole('option', { name: 'Ignored' }));

		expect(onchange).toHaveBeenCalledWith('ignored');
		expect(listbox()).toBeNull();
		expect(document.activeElement).toBe(button);
	});

	it('does not report the option already chosen', async () => {
		const { onchange, button } = renderSelect();

		await userEvent.click(button);
		await userEvent.click(screen.getByRole('option', { name: 'Resolved' }));

		expect(onchange).not.toHaveBeenCalled();
	});

	it('moves with the arrows, Home, End and letters, and chooses with Enter', async () => {
		const { onchange, button } = renderSelect('unresolved', 'All statuses');

		button.focus();
		await userEvent.keyboard('{ArrowDown}');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Unresolved' }));

		await userEvent.keyboard('{ArrowDown}');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Resolved' }));

		await userEvent.keyboard('{End}');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Ignored' }));

		await userEvent.keyboard('{Home}');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'All statuses' }));

		await userEvent.keyboard('{ArrowUp}');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'All statuses' }));

		await userEvent.keyboard('i');
		expect(document.activeElement).toBe(screen.getByRole('option', { name: 'Ignored' }));

		await userEvent.keyboard('{Enter}');
		expect(onchange).toHaveBeenCalledWith('ignored');
		expect(listbox()).toBeNull();
	});

	it('offers the option of no choice, whose value is empty', async () => {
		const { onchange, button } = renderSelect('resolved', 'All statuses');

		await userEvent.click(button);
		await userEvent.click(screen.getByRole('option', { name: 'All statuses' }));

		expect(onchange).toHaveBeenCalledWith('');
	});

	it('closes on Esc, which then does nothing else, and on a click outside', async () => {
		const { onchange, button } = renderSelect();

		await userEvent.click(button);

		const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
		document.activeElement?.dispatchEvent(escape);
		await Promise.resolve();

		expect(escape.defaultPrevented).toBe(true);
		expect(listbox()).toBeNull();
		expect(document.activeElement).toBe(button);

		await userEvent.click(button);
		expect(listbox()).not.toBeNull();

		await userEvent.click(document.body);
		expect(listbox()).toBeNull();
		expect(onchange).not.toHaveBeenCalled();
	});

	it('closes on scrolling or resizing back on the button when an option had the focus', async () => {
		const { button } = renderSelect();

		for (const event of [new Event('scroll'), new Event('resize')]) {
			await userEvent.click(button);
			expect(listbox()).not.toBeNull();

			(event.type === 'scroll' ? document.body : window).dispatchEvent(event);
			await Promise.resolve();

			expect(listbox()).toBeNull();
			expect(document.activeElement).toBe(button);
		}
	});
});

describe('Select as a form field', () => {
	it('shows its label above it and takes the width of the form', () => {
		render(Select, { label: 'Role', value: 'user', options, onchange: vi.fn(), field: true });

		const button = screen.getByRole('button', { name: /^Role/ });

		expect(screen.getByText('Role').classList.contains('sr-only')).toBe(false);
		expect(button.classList.contains('w-full')).toBe(true);
	});

	it('keeps the label for screen readers only in a filter bar', () => {
		render(Select, { label: 'Status', value: '', options, onchange: vi.fn() });

		expect(screen.getByText('Status').classList.contains('sr-only')).toBe(true);
	});
});
