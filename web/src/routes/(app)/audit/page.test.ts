import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import type { AuditEntry } from '#lib/audit.ts';
import type { PageProps } from './$types';
import Page from './+page.svelte';

const api = vi.hoisted(() => ({ get: vi.fn() }));
const navigation = vi.hoisted(() => ({ afterNavigate: [] as (() => void)[] }));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	afterNavigate: (callback: () => void) => navigation.afterNavigate.push(callback)
}));
vi.mock('$app/paths', () => ({
	resolve: (route: string, params: Record<string, string> = {}) =>
		route.replace('/(app)', '').replace(/\[id(=\w+)?\]/, params.id ?? '')
}));
vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://localhost/audit'),
		route: { id: '/(app)/audit' },
		data: { session: { can_administer: true, project_colors: {} } }
	},
	navigating: { to: null }
}));
vi.mock('#lib/api/client.ts', async (actual) => ({
	...(await actual<typeof import('#lib/api/client.ts')>()),
	get: api.get
}));

function entry(fields: Partial<AuditEntry>): AuditEntry {
	return {
		id: 1,
		created_at: '2026-11-25T12:00:00Z',
		user_id: 'k3x9q2mf',
		user_name: 'Ann',
		user_email: 'ann@example.com',
		source: 'ui',
		ip: '192.0.2.1',
		resource: 'issue',
		action: 'edit',
		record_id: 'aaaaaaaa',
		record_name: 'TypeError: x is undefined',
		project_ids: [1],
		projects: ['backend'],
		before: { status: 'unresolved' },
		after: { status: 'resolved' },
		...fields
	};
}

const grouped = entry({
	id: 2,
	record_id: '',
	record_name: '2 issues',
	item_count: 2,
	before: {},
	after: {}
});
const single = entry({ id: 3 });

function renderPage(entries: AuditEntry[], filters: Record<string, unknown> = {}) {
	const data = {
		filters: { resource: [], action: [], project: [], user: [], ...filters },
		choices: {
			resource: ['issue', 'user'],
			action: ['edit'],
			project: ['backend'],
			user: ['k3x9q2mf', 'a7c2d4e9', 'b8d3e5f0']
		},
		users: [
			{ id: 'k3x9q2mf', name: 'Ann', email: 'ann@example.com' },
			{ id: 'a7c2d4e9', name: 'Ann Lee', email: 'lee@example.com' },
			{ id: 'b8d3e5f0', name: 'Bob', email: 'bob@example.com' }
		],
		count: { count: entries.length },
		page: 1,
		perPage: 20,
		entries
	} as unknown as PageProps['data'];

	return render(Page, { data } as PageProps);
}

const dialog = () => screen.getByRole('dialog');

describe('the audit page', () => {
	it('opens a grouped entry in a dialog that reads its records', async () => {
		api.get.mockReset();
		api.get.mockResolvedValue({
			...grouped,
			items: [
				{
					record_id: 'aaaaaaaa',
					record_name: 'TypeError',
					project: 'backend',
					before: { status: 'unresolved' },
					after: { status: 'resolved' }
				},
				{
					record_id: 'bbbbbbbb',
					record_name: 'RangeError',
					project: 'backend',
					before: { status: 'ignored' },
					after: { status: 'resolved' }
				}
			]
		});
		renderPage([grouped]);

		await userEvent.click(screen.getByRole('button', { name: '2 issues' }));

		expect(api.get).toHaveBeenCalledWith('get_audit_entry', { id: 2 });
		await waitFor(() =>
			expect(within(dialog()).getByRole('table', { name: 'Changed records' })).toBeTruthy()
		);
		expect(within(dialog()).getByText('RangeError')).toBeTruthy();
		expect(within(dialog()).getByText('192.0.2.1')).toBeTruthy();
	});

	it('shows the change of one record without a request', async () => {
		api.get.mockReset();
		renderPage([single]);

		await userEvent.click(screen.getByRole('button', { name: 'Show the change' }));

		expect(api.get).not.toHaveBeenCalled();
		expect(within(dialog()).getByRole('rowheader', { name: 'status' })).toBeTruthy();
	});

	it('keeps the records of the entry shown when an earlier one answers late', async () => {
		api.get.mockReset();

		const answers: ((value: unknown) => void)[] = [];

		api.get.mockImplementation(() => new Promise((done) => answers.push(done)));

		const other = { ...grouped, id: 4, record_name: '3 issues', item_count: 3 };

		renderPage([grouped, other]);

		await userEvent.click(screen.getByRole('button', { name: '2 issues' }));
		await userEvent.click(screen.getByRole('button', { name: '3 issues' }));
		answers[1]({
			...other,
			items: [{ record_id: 'cccccccc', record_name: 'Newer', project: 'backend' }]
		});
		answers[0]({
			...grouped,
			items: [{ record_id: 'aaaaaaaa', record_name: 'Older', project: 'backend' }]
		});

		await waitFor(() => expect(within(dialog()).getByText('Newer')).toBeTruthy());
		expect(within(dialog()).queryByText('Older')).toBeNull();
	});

	it('closes the dialog after a navigation', async () => {
		navigation.afterNavigate.length = 0;
		renderPage([single]);

		await userEvent.click(screen.getByRole('button', { name: 'Show the change' }));
		expect(screen.getByRole('dialog')).toBeTruthy();

		for (const callback of navigation.afterNavigate) callback();
		await tick();

		expect(screen.queryByRole('dialog')).toBeNull();
	});

	it("names the user filter's chip after the user, not their id", () => {
		renderPage([single], { user: ['k3x9q2mf'] });

		const chip = screen.getByRole('link', { name: 'Remove the User filter' }).parentElement!;

		expect(chip.textContent).toContain('Ann');
		expect(chip.textContent).not.toContain('k3x9q2mf');
	});

	it('names a user who still exists by their current name, the old one in the tooltip', () => {
		renderPage([{ ...single, user_name: 'Ann Smith' }]);

		const link = screen.getAllByRole('link', { name: 'Ann' })[0];

		expect(link.getAttribute('title')).toBe('Named Ann Smith then');
	});

	it('narrows the Users filter to the names typed', async () => {
		renderPage([single]);

		// The button says All users, and so does the menu's first option.
		await fireEvent.click(screen.getAllByText('All users')[0]);
		await fireEvent.input(screen.getByRole('searchbox', { name: 'Find users' }), {
			target: { value: 'lee' }
		});

		const menu = screen.getByRole('searchbox', { name: 'Find users' }).closest('details')!;

		expect(menu.textContent).toContain('Ann Lee');
		expect(menu.textContent).not.toContain('Bob');
		expect(menu.textContent).not.toContain('example.com');
	});

	it('offers only the records and actions the server says some entry has', async () => {
		renderPage([single]);

		// One action is nothing to choose: its filter is hidden.
		expect(screen.queryByText('All actions')).toBeNull();

		await fireEvent.click(screen.getAllByText('All records')[0]);

		const menu = screen.getAllByText('All records')[0].closest('details')!;

		// All, then the two records.
		expect(within(menu).getAllByRole('checkbox').length).toBe(3);
	});
});
