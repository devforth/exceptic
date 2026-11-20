import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import ActiveFilters from './ActiveFilters.svelte';

describe('ActiveFilters', () => {
	it('links each value to the list without it, and Clear all', () => {
		render(ActiveFilters, {
			chips: [
				{
					label: 'Project',
					values: [
						{ text: 'backend', kind: 'project', remove: '/logs?project=worker' },
						{ text: 'worker', kind: 'project', remove: '/logs?project=backend' }
					]
				},
				{ label: 'Search', values: [{ text: 'timeout', remove: '/logs' }] }
			],
			clear: '/logs?per_page=50'
		});

		const remove = (name: string) =>
			screen.getByRole('link', { name: `Remove ${name}` }).getAttribute('href');

		expect(remove('Project: backend')).toBe('/logs?project=worker');
		expect(remove('Project: worker')).toBe('/logs?project=backend');
		expect(remove('Search: timeout')).toBe('/logs');
		expect(screen.getByRole('link', { name: 'Clear all' }).getAttribute('href')).toBe(
			'/logs?per_page=50'
		);
	});

	it('shows nothing without filters', () => {
		const { container } = render(ActiveFilters, { chips: [], clear: '/logs' });

		expect(container.textContent?.trim()).toBe('');
	});
});
