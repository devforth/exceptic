import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Pager from './Pager.svelte';

function renderPager(page: number, total: number, more = false) {
	const onperpage = vi.fn();
	const view = render(Pager, {
		page,
		perPage: 20,
		total,
		more,
		href: (number: number) => `/logs?page=${number}`,
		onperpage
	});

	return { onperpage, container: view.container };
}

const pageLinks = () =>
	screen.getAllByRole('link').map((link) => [link.textContent?.trim(), link.getAttribute('href')]);

describe('Pager', () => {
	it('shows the rows on the page, the first, last and nearby pages, and gaps', () => {
		const { container } = renderPager(6, 300, true);

		expect(screen.getByText('101–120 of 300+')).toBeTruthy();
		expect(pageLinks()).toEqual([
			['← Prev', '/logs?page=5'],
			['1', '/logs?page=1'],
			['4', '/logs?page=4'],
			['5', '/logs?page=5'],
			['7', '/logs?page=7'],
			['8', '/logs?page=8'],
			['15', '/logs?page=15'],
			['Next →', '/logs?page=7']
		]);
		expect(container.querySelector('[aria-current="page"]')?.textContent?.trim()).toBe('6');
		expect(screen.getAllByText('…')).toHaveLength(2);
	});

	it('has no previous link on the first page and no next link on the last', () => {
		renderPager(1, 30);

		expect(pageLinks()).toEqual([
			['2', '/logs?page=2'],
			['Next →', '/logs?page=2']
		]);
	});

	it('changes the page size', async () => {
		const { onperpage } = renderPager(1, 30);

		await userEvent.click(screen.getByRole('button', { name: 'Rows per page 20 per page' }));
		await userEvent.click(screen.getByRole('option', { name: '50 per page' }));

		expect(onperpage).toHaveBeenCalledWith(50);
	});

	it('is not shown without rows', () => {
		const { container } = renderPager(1, 0);

		expect(container.querySelector('nav')).toBeNull();
	});
});
