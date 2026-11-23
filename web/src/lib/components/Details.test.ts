import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Details from './Details.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>Inside</p>' }));
const summary = createRawSnippet(() => ({ render: () => '<span>error, warning</span>' }));

describe('Details', () => {
	it('starts closed and opens from its heading, with its count', async () => {
		const { container } = render(Details, { title: 'Breadcrumbs', count: 12, children });
		const details = container.querySelector('details')!;

		expect(details.open).toBe(false);
		expect(screen.getByRole('heading', { name: 'Breadcrumbs' })).toBeTruthy();
		expect(screen.getByText('12')).toBeTruthy();

		await userEvent.click(screen.getByText('Breadcrumbs'));
		expect(details.open).toBe(true);
	});

	it('opens a form section whose value was refused', () => {
		const { container } = render(Details, {
			title: 'Levels to record',
			variant: 'form',
			hint: 'Levels stored for this project.',
			summary,
			invalid: true,
			children
		});

		expect(container.querySelector('details')!.open).toBe(true);
		expect(screen.getByText('error, warning')).toBeTruthy();
		expect(screen.getByRole('img', { name: 'Levels stored for this project.' })).toBeTruthy();
	});

	it('stays closed while its values are accepted', () => {
		const { container } = render(Details, { title: 'Levels', variant: 'form', children });

		expect(container.querySelector('details')!.open).toBe(false);
	});

	it('draws a nested box with its title and what its count counts, without a heading', () => {
		render(Details, {
			title: 'Stack trace',
			variant: 'nested',
			count: 10,
			unit: 'frames',
			children
		});

		expect(screen.getByText('Stack trace')).toBeTruthy();
		expect(screen.getByText('10 frames')).toBeTruthy();
		expect(screen.queryByRole('heading')).toBeNull();
	});
});
