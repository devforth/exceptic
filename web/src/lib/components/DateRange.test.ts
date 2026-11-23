import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DateRange from './DateRange.svelte';

beforeEach(() => {
	vi.useFakeTimers({ toFake: ['Date'] });
	vi.setSystemTime(new Date('2026-11-23T12:00:00Z'));
});

afterEach(() => {
	vi.useRealTimers();
});

function renderRange(from = '', to = '') {
	const onchange = vi.fn();

	render(DateRange, { from, to, onchange });

	return { onchange, field: screen.getByRole('button', { name: /^Date range/ }) };
}

const day = (name: string) => screen.getByRole('button', { name });
const calendar = () => screen.queryByRole('dialog', { name: 'Choose dates' });

describe('DateRange', () => {
	it('shows the range in the field, or what it is for', () => {
		renderRange('2026-11-20', '2026-11-23');

		expect(screen.getByRole('button', { name: 'Date range: 20 Nov – 23 Nov' })).toBeTruthy();
	});

	it('picks the first and the last day with two clicks, swapping a reversed range', async () => {
		const { onchange, field } = renderRange();

		await userEvent.click(field);
		expect(field.getAttribute('aria-expanded')).toBe('true');
		expect(screen.getByText('November 2026')).toBeTruthy();
		expect(document.activeElement).toBe(day('Monday, 23 November 2026'));

		await userEvent.click(day('Friday, 20 November 2026'));
		expect(onchange).not.toHaveBeenCalled();

		await userEvent.click(day('Tuesday, 10 November 2026'));
		expect(onchange).toHaveBeenCalledWith('2026-11-10', '2026-11-20');
		expect(calendar()).toBeNull();
		expect(document.activeElement).toBe(field);
	});

	it('joins the chosen days in one band that runs under the first and last day', async () => {
		const { field } = renderRange('2026-11-18', '2026-11-20');

		await userEvent.click(field);

		const band = (name: string) => day(name).closest('td')?.dataset.band;

		expect(band('Wednesday, 18 November 2026')).toBe('start');
		expect(band('Thursday, 19 November 2026')).toBe('between');
		expect(band('Friday, 20 November 2026')).toBe('end');
		expect(band('Saturday, 21 November 2026')).toBeUndefined();

		// One shape: the first day rounded on its left, the last on its right, square between.
		expect(day('Wednesday, 18 November 2026').classList.contains('rounded-l-full')).toBe(true);
		expect(day('Wednesday, 18 November 2026').classList.contains('rounded-r-full')).toBe(false);
		expect(day('Friday, 20 November 2026').classList.contains('rounded-r-full')).toBe(true);
		expect(day('Friday, 20 November 2026').classList.contains('rounded-l-full')).toBe(false);
		expect(day('Thursday, 19 November 2026').classList.contains('rounded-full')).toBe(false);
	});

	it('keeps the pointer on every day that can be picked, the chosen ones too', async () => {
		const { field } = renderRange('2026-11-18', '2026-11-20');

		await userEvent.click(field);

		for (const name of [
			'Wednesday, 18 November 2026',
			'Thursday, 19 November 2026',
			'Friday, 20 November 2026',
			'Saturday, 21 November 2026'
		]) {
			expect(day(name).classList.contains('cursor-pointer')).toBe(true);
		}
	});

	it('offers no day after today', async () => {
		const { field } = renderRange();

		await userEvent.click(field);

		expect(day('Tuesday, 24 November 2026').hasAttribute('disabled')).toBe(true);
		expect(screen.getByRole('button', { name: 'Next month' }).hasAttribute('disabled')).toBe(true);
	});

	it('moves by the keyboard and chooses with Enter', async () => {
		const { onchange, field } = renderRange('2026-10-15', '2026-10-20');

		await userEvent.click(field);
		expect(screen.getByText('October 2026')).toBeTruthy();
		expect(document.activeElement).toBe(day('Thursday, 15 October 2026'));

		await userEvent.keyboard('{ArrowRight}');
		expect(document.activeElement).toBe(day('Friday, 16 October 2026'));

		await userEvent.keyboard('{ArrowDown}');
		expect(document.activeElement).toBe(day('Friday, 23 October 2026'));

		await userEvent.keyboard('{Home}');
		expect(document.activeElement).toBe(day('Monday, 19 October 2026'));

		await userEvent.keyboard('{Enter}');
		await userEvent.keyboard('{PageDown}');
		expect(screen.getByText('November 2026')).toBeTruthy();
		expect(document.activeElement).toBe(day('Thursday, 19 November 2026'));

		await userEvent.keyboard('{PageDown}');
		expect(document.activeElement).toBe(day('Monday, 23 November 2026'));

		await userEvent.keyboard('{Enter}');
		expect(onchange).toHaveBeenCalledWith('2026-10-19', '2026-11-23');
	});

	it('closes on Esc, which then does nothing else, and on a click outside', async () => {
		const { onchange, field } = renderRange();

		await userEvent.click(field);

		const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
		document.activeElement?.dispatchEvent(escape);
		await Promise.resolve();

		expect(escape.defaultPrevented).toBe(true);
		expect(calendar()).toBeNull();
		expect(document.activeElement).toBe(field);

		await userEvent.click(field);
		await userEvent.click(document.body);
		expect(calendar()).toBeNull();
		expect(onchange).not.toHaveBeenCalled();
	});

	it('clears both days with ×', async () => {
		const { onchange } = renderRange('2026-11-20', '2026-11-23');

		await userEvent.click(screen.getByRole('button', { name: 'Clear dates' }));

		expect(onchange).toHaveBeenCalledWith('', '');
	});

	it('has nothing to clear without days', () => {
		renderRange();

		expect(screen.queryByRole('button', { name: 'Clear dates' })).toBeNull();
	});

	it('closes on Esc while the field or × has the focus', async () => {
		const { field } = renderRange('2026-11-20', '2026-11-23');

		for (const target of [field, screen.getByRole('button', { name: 'Clear dates' })]) {
			await userEvent.click(field);
			expect(calendar()).not.toBeNull();
			target.focus();

			const escape = new KeyboardEvent('keydown', {
				key: 'Escape',
				bubbles: true,
				cancelable: true
			});
			target.dispatchEvent(escape);
			await Promise.resolve();

			expect(escape.defaultPrevented).toBe(true);
			expect(calendar()).toBeNull();
			expect(document.activeElement).toBe(field);
		}
	});
});
