import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import UserForm from './UserForm.svelte';
import type { UserInput } from './users.ts';

vi.mock('$app/state', () => ({ page: { data: {} } }));

const projects = [
	{ id: 1, name: 'backend' },
	{ id: 2, name: 'frontend' }
];

const checkbox = (name: string | RegExp) =>
	screen.getByRole<HTMLInputElement>('checkbox', { name });

const saved = {
	email: 'alice@example.com',
	name: 'Alice',
	role: 'user',
	allProjects: false,
	projectIds: [1],
	accessEditable: true
};

describe('UserForm', () => {
	it('asks no projects of an admin, who has every one', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();

		render(UserForm, {
			projects,
			viewerAll: true,
			roles: ['admin', 'user'],
			submitLabel: 'Add user',
			save
		});

		expect(checkbox(/All projects/)).toBeTruthy();
		await user.click(screen.getByRole('radio', { name: /^admin/ }));
		expect(screen.queryByRole('checkbox', { name: /All projects/ })).toBeNull();

		await user.type(screen.getByLabelText(/^Email/), 'ops@example.com');
		await user.click(screen.getByRole('button', { name: 'Add user' }));

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith({ email: 'ops@example.com', name: '', role: 'admin' })
		);
	});

	it('starts a new user with the projects of a limited viewer, All not offered', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();

		render(UserForm, {
			projects,
			viewerAll: false,
			roles: ['admin', 'user'],
			submitLabel: 'Add user',
			save
		});

		expect(checkbox(/All projects/).checked).toBe(false);
		expect(checkbox(/All projects/).disabled).toBe(true);
		expect(checkbox('backend').checked).toBe(true);
		expect(checkbox('frontend').checked).toBe(true);

		await user.type(screen.getByLabelText(/^Email/), 'bob@example.com');
		await user.click(checkbox('frontend'));
		await user.click(screen.getByRole('button', { name: 'Add user' }));

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith({
				email: 'bob@example.com',
				name: '',
				role: 'user',
				access: { all_projects: false, projects: ['1'] }
			})
		);
	});

	it('starts a new user at All projects for a viewer who has all', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();

		render(UserForm, {
			projects,
			viewerAll: true,
			roles: ['admin', 'user'],
			submitLabel: 'Add user',
			save
		});

		expect(checkbox(/All projects/).checked).toBe(true);
		expect(checkbox(/All projects/).disabled).toBe(false);

		await user.type(screen.getByLabelText(/^Email/), 'bob@example.com');
		await user.click(screen.getByRole('button', { name: 'Add user' }));

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith(
				expect.objectContaining({ access: { all_projects: true, projects: [] } })
			)
		);
	});

	it('sends a saved user only once changed, the projects counted as a change', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();
		const button = () => screen.getByRole('button', { name: 'Save' });

		render(UserForm, {
			projects,
			viewerAll: false,
			roles: ['admin', 'user'],
			user: saved,
			submitLabel: 'Save',
			save
		});

		expect(checkbox(/^backend/).checked).toBe(true);
		expect(checkbox('frontend').checked).toBe(false);
		expect(button().getAttribute('aria-disabled')).toBe('true');

		await user.click(checkbox('frontend'));
		expect(button().getAttribute('aria-disabled')).toBeNull();
		await user.click(button());

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith({
				email: 'alice@example.com',
				name: 'Alice',
				role: 'user',
				access: { all_projects: false, projects: ['1', '2'] }
			})
		);
	});

	it('locks the projects of a user with all of them for a limited viewer and leaves them out', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();
		const reason = 'Only someone with access to all projects can change this.';

		render(UserForm, {
			projects,
			viewerAll: false,
			roles: ['admin', 'user'],
			user: {
				...saved,
				allProjects: true,
				projectIds: [],
				accessEditable: false,
				accessReason: reason
			},
			submitLabel: 'Save',
			save
		});

		expect(checkbox(/All projects/).checked).toBe(true);
		expect(checkbox(/All projects/).disabled).toBe(true);
		expect(screen.getByText(new RegExp(reason))).toBeTruthy();

		await user.clear(screen.getByLabelText(/^Name/));
		await user.type(screen.getByLabelText(/^Name/), 'Alice B');
		await user.click(screen.getByRole('button', { name: 'Save' }));

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith({
				email: 'alice@example.com',
				name: 'Alice B',
				role: 'user'
			})
		);
	});
});

describe('UserForm take over', () => {
	it('keeps the email, name and role of a user the viewer may not take over, saying why', async () => {
		const save = vi.fn((input: UserInput) => Promise.resolve(input));
		const user = userEvent.setup();
		const reason = "Only an admin with all of this user's projects can change it.";

		render(UserForm, {
			projects,
			viewerAll: false,
			roles: ['admin', 'user'],
			user: { ...saved, takeOverLocked: reason },
			submitLabel: 'Save',
			save
		});

		const radios = screen.getAllByRole<HTMLInputElement>('radio');

		expect(radios.every((radio) => radio.disabled)).toBe(true);
		expect(screen.getByText(`(${reason})`)).toBeTruthy();
		expect(screen.getByLabelText<HTMLInputElement>(/^Email/).readOnly).toBe(true);
		expect(screen.getByLabelText<HTMLInputElement>(/^Name/).readOnly).toBe(true);
		expect(screen.getByText(reason)).toBeTruthy();

		// Their projects the viewer sees still change.
		await user.click(checkbox('frontend'));
		await user.click(screen.getByRole('button', { name: 'Save' }));

		await waitFor(() =>
			expect(save).toHaveBeenCalledWith({
				email: 'alice@example.com',
				name: 'Alice',
				role: 'user',
				access: { all_projects: false, projects: ['1', '2'] }
			})
		);
	});
});
