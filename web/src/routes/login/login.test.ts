import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import LoginPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ send: vi.fn(), assign: vi.fn() }));

vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<object>()),
	send: mocks.send
}));
vi.mock('$app/state', () => ({
	page: { url: new URL('http://localhost/login?next=%2Flogs'), data: {} }
}));

beforeEach(() => vi.stubGlobal('location', { assign: mocks.assign }));

afterEach(() => {
	vi.unstubAllGlobals();
	mocks.send.mockReset();
	mocks.assign.mockReset();
});

function renderLogin(disabled?: string) {
	render(LoginPage, { data: { state: { app_name: 'Exceptic', disabled } } } as never);
}

const field = (name: string) => screen.getByLabelText<HTMLInputElement>(name);

describe('the login page', () => {
	it('opens the page the server names after a login', async () => {
		mocks.send.mockResolvedValue({ next: '/logs' });
		renderLogin();

		expect(document.activeElement).toBe(field('Email'));

		await userEvent.type(field('Email'), 'admin');
		await userEvent.type(field('Password'), 'correct horse battery');
		await userEvent.click(screen.getByRole('button', { name: 'Log in' }));

		expect(mocks.send).toHaveBeenCalledWith('login', {
			email: 'admin',
			password: 'correct horse battery',
			next: '/logs'
		});
		await waitFor(() => expect(mocks.assign).toHaveBeenCalledWith('/logs'));
	});

	it('shows a refusal, keeps the email and clears the password', async () => {
		mocks.send.mockRejectedValue(new ApiError(401, 'Wrong email or password.'));
		renderLogin();

		await userEvent.type(field('Email'), 'admin');
		await userEvent.type(field('Password'), 'wrong password');
		await userEvent.click(screen.getByRole('button', { name: 'Log in' }));

		expect((await screen.findByRole('alert')).textContent).toContain('Wrong email or password.');
		expect(field('Email').value).toBe('admin');
		expect(field('Password').value).toBe('');
		expect(document.activeElement).toBe(field('Password'));
		expect(mocks.assign).not.toHaveBeenCalled();
	});

	it('shows the setup form after the env login, then opens the app', async () => {
		mocks.send.mockResolvedValueOnce({ next: '/logs', setup: true });
		renderLogin();

		await userEvent.type(field('Email'), 'admin');
		await userEvent.type(field('Password'), 'env password 123');
		await userEvent.click(screen.getByRole('button', { name: 'Log in' }));

		await screen.findByRole('button', { name: 'Create admin' });
		expect(screen.getByText(/This creates the first admin/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Log in' })).toBeNull();
		expect(mocks.assign).not.toHaveBeenCalled();

		mocks.send.mockRejectedValueOnce(
			new ApiError(400, 'The passwords do not match.', {
				fields: { confirm_password: 'The passwords do not match.' }
			})
		);
		await userEvent.type(field('Email'), 'owner@example.com');
		await userEvent.type(field('Password'), 'owner password 1');
		await userEvent.type(field('Repeat the password'), 'owner password 2');
		await userEvent.click(screen.getByRole('button', { name: 'Create admin' }));

		expect(await screen.findByText('The passwords do not match.')).toBeTruthy();
		expect(mocks.assign).not.toHaveBeenCalled();

		mocks.send.mockResolvedValueOnce(undefined);
		await userEvent.clear(field('Repeat the password'));
		await userEvent.type(field('Repeat the password'), 'owner password 1');
		await userEvent.click(screen.getByRole('button', { name: 'Create admin' }));

		expect(mocks.send).toHaveBeenLastCalledWith('setup_admin', {
			email: 'owner@example.com',
			password: 'owner password 1',
			confirm_password: 'owner password 1'
		});
		await waitFor(() => expect(mocks.assign).toHaveBeenCalledWith('/'));
	});

	it('says why nobody can log in without an admin', () => {
		renderLogin('The web UI is disabled: set ADMIN_PASSWORD to enable it.');

		expect(screen.getByText(/The web UI is disabled/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Log in' })).toBeNull();
	});
});
