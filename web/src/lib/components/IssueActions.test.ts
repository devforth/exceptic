import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import IssueActions from './IssueActions.svelte';
import {
	actionsFor,
	confirmationOf,
	doneMessage,
	selectionProblem,
	type IssueAction
} from './issueActions.ts';
import StatusBadge from './StatusBadge.svelte';
import { statusRowTone, statusTitle } from './issueStatus.ts';

describe('the actions an issue offers', () => {
	it('follow its status, and Delete only for who may delete', () => {
		expect(actionsFor('unresolved', false)).toEqual(['resolve', 'ignore']);
		expect(actionsFor('resolved', true)).toEqual(['reopen', 'delete']);
		expect(actionsFor('ignored', false)).toEqual(['reopen']);
		expect(actionsFor(undefined, true)).toEqual(['resolve', 'ignore', 'reopen', 'delete']);
	});

	it('ask about this issue or the checked ones', () => {
		expect(confirmationOf('resolve').message).toBe(
			'Resolve this issue? If the error happens again, it is reopened.'
		);
		expect(confirmationOf('delete', 3)).toMatchObject({
			title: 'Delete issues',
			message:
				'Delete 3 selected issues and all its events? This cannot be undone. If the error happens again, it starts a new issue.',
			confirmLabel: 'Delete',
			tone: 'danger'
		});
		expect(confirmationOf('reopen', 1).message).toBe('Reopen 1 selected issue?');
	});

	it('say what was done, and why nothing can be', () => {
		expect(doneMessage('ignore', 1)).toBe('Issue ignored.');
		expect(doneMessage('resolve', 4)).toBe('4 issues resolved.');
		expect(selectionProblem(0)).toBe('Select issues to change them together');
		expect(selectionProblem(1001)).toBe('At most 1000 issues at once');
		expect(selectionProblem(20)).toBeUndefined();
	});
});

describe('issue statuses', () => {
	it('say what they mean and tint their rows', () => {
		expect(statusTitle('resolved')).toBe('Resolved: fixed, reopens if the error happens again');
		expect(statusTitle('all')).toBeUndefined();
		expect(statusRowTone('unresolved')).toBe('error');
		expect(statusRowTone('ignored')).toBe('faded');
	});

	it('show their sign to the eye only', () => {
		const { container } = render(StatusBadge, { status: 'unresolved', href: '/issues' });
		const link = screen.getByRole('link');

		expect(link.textContent?.trim()).toBe('●unresolved');
		expect(link.getAttribute('title')).toBe('Unresolved: still happening or not looked at yet');
		expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe('●');
	});
});

describe('IssueActions', () => {
	const fetchMock = vi.fn<(address: string, init: RequestInit) => Promise<Response>>(() =>
		Promise.resolve(new Response('{"action":"resolve","issues":2}'))
	);

	afterEach(() => {
		fetchMock.mockClear();
		vi.unstubAllGlobals();
	});

	function renderActions(props: { issueIds: string[]; status?: string; canDelete?: boolean }) {
		vi.stubGlobal('fetch', fetchMock);

		const ondone = vi.fn<(action: IssueAction) => void>();

		render(IssueActions, { canDelete: false, ondone, ...props });

		return ondone;
	}

	it('hides Delete from who may not delete', () => {
		renderActions({ issueIds: ['issue001'], status: 'unresolved' });

		expect(screen.getByRole('button', { name: 'Resolve' })).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Reopen' })).toBeNull();
	});

	it('is off while no issue is checked', () => {
		renderActions({ issueIds: [], canDelete: true });

		for (const name of ['Resolve', 'Ignore', 'Reopen', 'Delete']) {
			expect(screen.getByRole<HTMLButtonElement>('button', { name }).disabled).toBe(true);
		}
	});

	it('sends nothing when cancelled, the action once confirmed', async () => {
		const ondone = renderActions({ issueIds: ['issue001', 'issue002'] });

		await userEvent.click(screen.getByRole('button', { name: 'Resolve' }));
		expect(screen.getByText(/Resolve 2 selected issues\?/)).toBeTruthy();
		await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(fetchMock).not.toHaveBeenCalled();

		await userEvent.click(screen.getByRole('button', { name: 'Resolve' }));
		const dialog = screen.getByRole('dialog');
		const confirm = [...dialog.querySelectorAll('button')].find(
			(button) => button.textContent?.trim() === 'Resolve'
		)!;

		await userEvent.click(confirm);
		await waitFor(() => expect(ondone).toHaveBeenCalledWith('resolve'));

		const [address, init] = fetchMock.mock.calls[0];

		expect(address).toBe('/api/ui/apply_issue_action');
		expect(JSON.parse(init.body as string)).toEqual({
			action: 'resolve',
			issue_ids: ['issue001', 'issue002']
		});
	});
});
