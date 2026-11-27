import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import ProjectsSummary from './ProjectsSummary.svelte';

vi.mock('$app/paths', () => ({
	resolve: (route: string, params?: { id?: number }) =>
		route.replace('/(app)', '').replace('[id=projectId]', String(params?.id ?? ''))
}));
vi.mock('$app/state', () => ({ page: { data: {} } }));

const names = ['backend', 'frontend', 'worker', 'mobile'];

describe('ProjectsSummary', () => {
	it("links each badge to its project's settings from ids", () => {
		render(ProjectsSummary, { all: false, names: ['backend'], ids: [7] });

		expect(screen.getByRole('link', { name: 'backend' }).getAttribute('href')).toBe('/projects/7');
	});

	it('keeps a row to limit badges, the rest behind "+N more" in a popup', async () => {
		const { container } = render(ProjectsSummary, { all: false, names, limit: 2 });

		// On a phone only the first stays in the row: its "+3 more" is hidden from the table width up.
		const phone = screen.getByRole('button', { name: '+3 more' });
		const wide = screen.getByRole('button', { name: '+2 more' });

		expect(phone.closest('.table\\:hidden')).toBeTruthy();
		expect(wide.closest('.max-table\\:hidden')).toBeTruthy();
		expect(container.querySelector('.max-table\\:hidden')?.textContent).toContain('frontend');

		await fireEvent.click(wide);

		const popup = screen.getByRole('dialog', { name: '2 more projects' });

		expect(popup.textContent).toContain('worker');
		expect(popup.textContent).toContain('mobile');
		expect(popup.textContent).not.toContain('backend');
		expect(wide.getAttribute('aria-expanded')).toBe('true');
	});

	it('shows every badge without a limit', () => {
		render(ProjectsSummary, { all: false, names });

		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.getByText('mobile')).toBeTruthy();
	});
});
