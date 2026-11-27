import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import AuditItems from './AuditItems.svelte';

const resolved = [
	{
		record_id: 'aaaaaaaa',
		record_name: 'TypeError: x is undefined',
		project: 'backend',
		before: { status: 'unresolved' },
		after: { status: 'resolved' },
		href: '/issues/aaaaaaaa'
	},
	{
		record_id: 'bbbbbbbb',
		record_name: 'RangeError: bad length',
		project: 'frontend',
		before: { status: 'ignored' },
		after: { status: 'resolved' }
	}
];

const dataRows = () =>
	screen.getByRole('table', { name: 'Changed records' }).querySelectorAll('tbody tr');

describe('AuditItems', () => {
	it('shows what every record changed alike once and what differs as a column, rows tinted by the status before', () => {
		render(AuditItems, { items: resolved });

		const common = screen.getByRole('rowheader', { name: 'status' }).closest('tr')!;

		expect(common.querySelector('[data-side="after"]')?.textContent).toContain('resolved');
		expect(common.querySelector('[data-side="before"]')).toBeNull();
		expect(
			within(screen.getByRole('table', { name: 'Changed records' }))
				.getAllByRole('columnheader')
				.map((heading) => heading.textContent?.trim())
		).toEqual(['Project', 'Record', 'status before']);

		const [first, second] = dataRows();

		expect(first.textContent).toContain('unresolved');
		expect(first.className).toContain('bg-row-error');
		expect(second.textContent).toContain('ignored');
		expect(second.className).toContain('opacity-50');
	});

	it('links a record only when it has a page', () => {
		render(AuditItems, { items: resolved });

		expect(screen.getByRole('link', { name: /TypeError/ }).getAttribute('href')).toBe(
			'/issues/aaaaaaaa'
		);
		expect(screen.queryByRole('link', { name: /RangeError/ })).toBeNull();
		expect(screen.getByText('RangeError: bad length')).toBeTruthy();
	});

	it('puts the records the search finds first, the text it found marked', () => {
		render(AuditItems, { items: resolved, query: 'range' });

		const [first] = dataRows();

		expect(first.textContent).toContain('RangeError');
		expect(first.querySelector('mark')?.textContent?.trim()).toBe('Range');
	});

	it('says when the viewer sees none of the records', () => {
		render(AuditItems, { items: [] });

		expect(screen.getByText('No record you can see.')).toBeTruthy();
	});
});
