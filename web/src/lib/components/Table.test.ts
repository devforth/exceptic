import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import TableSelection from '../../test/TableSelection.svelte';
import Table from './Table.svelte';
import { columnSort, type ColumnSort, type TableColumn } from './table.ts';

type Issue = { id: string; title: string; status: 'unresolved' | 'resolved' };

const issues: Issue[] = [
	{ id: 'a', title: 'TypeError', status: 'unresolved' },
	{ id: 'b', title: 'RangeError', status: 'resolved' },
	{ id: 'c', title: 'SyntaxError', status: 'unresolved' }
];

const columns: TableColumn<'title' | 'last_seen'>[] = [
	{ key: 'title', label: 'Issue', rowHeader: true },
	{
		key: 'last_seen',
		label: 'Last seen',
		sort: { ascending: 'last_seen_asc', descending: 'last_seen', first: 'descending' }
	}
];

const cell = createRawSnippet((row: () => Issue) => ({
	render: () => `<span>${row().title}</span>`
}));

function renderTable(props: Record<string, unknown> = {}) {
	return render(Table<Issue, 'title' | 'last_seen'>, {
		columns,
		rows: issues,
		cell,
		rowKey: (issue: Issue) => issue.id,
		label: 'Issues',
		...props
	});
}

const boxes = () => screen.getAllByRole<HTMLInputElement>('checkbox');

describe('Table', () => {
	it('links a sortable heading to the list sorted by it, the reverse once sorted', () => {
		const { rerender } = renderTable({
			sort: { current: 'project', href: (sort: string) => `?sort=${sort}` }
		});

		let heading = screen.getByRole('columnheader', { name: 'Last seen' });
		expect(heading.getAttribute('aria-sort')).toBeNull();
		expect(screen.getByRole('link', { name: 'Last seen' }).getAttribute('href')).toBe(
			'?sort=last_seen'
		);

		void rerender({ sort: { current: 'last_seen', href: (sort: string) => `?sort=${sort}` } });
		heading = screen.getByRole('columnheader', { name: 'Last seen' });
		expect(heading.getAttribute('aria-sort')).toBe('descending');
	});

	it('names each row by its row header', () => {
		renderTable();

		expect(screen.getByRole('rowheader', { name: 'TypeError' })).toBeTruthy();
	});

	it('selects rows one by one or all, the box for all mixed in between', async () => {
		renderTable({ selected: new Set(), selectLabel: (issue: Issue) => `Select ${issue.title}` });

		const [all] = boxes();

		await userEvent.click(screen.getByRole('checkbox', { name: 'Select RangeError' }));
		expect(all.indeterminate).toBe(true);
		expect(all.checked).toBe(false);

		await userEvent.click(all);
		expect(boxes().every((box) => box.checked)).toBe(true);
		expect(all.indeterminate).toBe(false);

		await userEvent.click(all);
		expect(boxes().some((box) => box.checked)).toBe(false);
	});

	it('empties the selection when the rows change', async () => {
		render(TableSelection);

		await userEvent.click(screen.getByRole('checkbox', { name: 'Select all' }));
		expect(screen.getByText('Checked: 1, 2')).toBeTruthy();

		await userEvent.click(screen.getByRole('button', { name: 'Other rows' }));
		expect(screen.getByText('Checked: none')).toBeTruthy();
		expect(boxes().some((box) => box.checked)).toBe(false);
	});

	it('tints rows by their tone, titles them and marks the selected one', () => {
		renderTable({
			rowTone: (issue: Issue) => (issue.status === 'unresolved' ? 'error' : 'success'),
			rowTitle: (issue: Issue) => issue.status,
			selectedKey: 'c'
		});

		const [, first, second, third] = screen.getAllByRole('row');

		expect(first.className).toContain('bg-row-error');
		expect(second.className).toContain('bg-row-success');
		expect(second.title).toBe('resolved');
		expect(third.getAttribute('aria-current')).toBe('true');
		expect(third.className).toContain('bg-selected');
	});
});

describe('columnSort', () => {
	const lastSeen: ColumnSort = {
		ascending: 'last_seen_asc',
		descending: 'last_seen',
		first: 'descending'
	};
	const project: ColumnSort = {
		ascending: 'project',
		descending: 'project_desc',
		first: 'ascending'
	};

	it('links an unsorted column to its first direction', () => {
		expect(columnSort(lastSeen, 'project')).toEqual({ next: 'last_seen' });
		expect(columnSort(project, 'last_seen')).toEqual({ next: 'project' });
	});

	it('links a sorted column to the reverse', () => {
		expect(columnSort(lastSeen, 'last_seen')).toEqual({
			direction: 'descending',
			next: 'last_seen_asc'
		});
		expect(columnSort(lastSeen, 'last_seen_asc')).toEqual({
			direction: 'ascending',
			next: 'last_seen'
		});
	});

	it('marks the rows busy while they load again', () => {
		const { container } = renderTable({ busy: true });

		expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
		expect(screen.getByRole('table').classList.contains('opacity-50')).toBe(true);
	});

	it('is not busy by default', () => {
		const { container } = renderTable();

		expect(container.querySelector('[aria-busy]')).toBeNull();
	});
});
