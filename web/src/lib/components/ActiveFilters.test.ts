import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import ActiveFilters from './ActiveFilters.svelte';

describe('ActiveFilters', () => {
	const chips = [
		{
			label: 'Project',
			remove: '/logs',
			values: [
				{ text: 'backend', kind: 'project' as const, remove: '/logs?project=worker' },
				{ text: 'worker', kind: 'project' as const, remove: '/logs?project=backend' }
			]
		},
		{
			label: 'Search',
			remove: '/logs?project=backend',
			values: [{ text: 'timeout', remove: '/logs?project=backend' }]
		}
	];

	const href = (name: string) => screen.getByRole('link', { name }).getAttribute('href');

	it('removes one of several values by its badge, the whole filter after the chip', () => {
		render(ActiveFilters, { chips, clear: '/logs?per_page=50' });

		expect(href('Remove Project: backend')).toBe('/logs?project=worker');
		expect(href('Remove Project: worker')).toBe('/logs?project=backend');
		expect(href('Remove the Project filter')).toBe('/logs');
		expect(href('Remove the Search filter')).toBe('/logs?project=backend');
		expect(href('Clear all')).toBe('/logs?per_page=50');
		// The whole badge is the link, its name and the × inside it.
		const badge = screen.getByRole('link', { name: 'Remove Project: backend' });

		expect(badge.textContent).toContain('backend');
		expect(badge.textContent).toContain('×');
	});

	it('gives a single value no × of its own: the filter’s does the same', () => {
		render(ActiveFilters, { chips: [chips[1]], clear: '/logs' });

		expect(screen.queryByRole('link', { name: 'Remove Search: timeout' })).toBeNull();
		expect(screen.getAllByRole('link', { name: /^Remove/ })).toHaveLength(1);
	});

	it('shows nothing without filters', () => {
		const { container } = render(ActiveFilters, { chips: [], clear: '/logs' });

		expect(container.textContent?.trim()).toBe('');
	});
});
