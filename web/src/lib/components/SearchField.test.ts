import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import SearchField from './SearchField.svelte';

describe('SearchField', () => {
	it('reports every change of its text, and Enter to load at once', async () => {
		const onchange = vi.fn();

		render(SearchField, { value: '', label: 'Search', onchange });
		await userEvent.type(screen.getByRole('searchbox'), 'ab{Enter}');

		expect(onchange.mock.calls).toEqual([
			['a', false],
			['ab', false],
			['ab', true]
		]);
	});
});

describe('SearchField clear', () => {
	it('shows a × only with text, which clears it at once', async () => {
		const onchange = vi.fn();
		const { rerender } = render(SearchField, { value: '', label: 'Search', onchange });

		expect(screen.queryByRole('button', { name: 'Clear Search' })).toBeNull();

		await rerender({ value: 'timeout', label: 'Search', onchange });
		await userEvent.click(screen.getByRole('button', { name: 'Clear Search' }));

		expect(onchange).toHaveBeenLastCalledWith('', true);
		expect(document.activeElement).toBe(screen.getByRole('searchbox'));
	});
});
