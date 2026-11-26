import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import AccountPage from './+page.svelte';

const mocks = vi.hoisted(() => ({ send: vi.fn(), invalidate: vi.fn() }));

vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<object>()),
	send: mocks.send
}));
vi.mock('$app/navigation', () => ({ invalidate: mocks.invalidate }));

afterEach(() => {
	mocks.send.mockReset();
	mocks.invalidate.mockReset();
});

function renderAccount(user: { email: string; must_change_password?: boolean }) {
	const session = { user: { display_name: 'Root', role: 'admin', ...user } };

	render(AccountPage, { data: { session } } as never);
}

// The Security row opens the dialog; its name carries the email under it.
async function openEmail(user: ReturnType<typeof userEvent.setup>) {
	await user.click(screen.getByRole('button', { name: /^Change email .+/ }));
}

// The open dialog: both are in the page, closed ones hidden.
const dialog = () => within(screen.getByRole('dialog'));

const submit = () => screen.getByRole('button', { name: /^(Change email|Nothing changed)$/ });

describe('the account email', () => {
	it('sends a changed email with the current password from its dialog', async () => {
		mocks.send.mockResolvedValue(undefined);
		const user = userEvent.setup();
		renderAccount({ email: 'admin' });

		await openEmail(user);

		const email = dialog().getByLabelText<HTMLInputElement>(/^Email/);

		expect(email.value).toBe('admin');

		await user.clear(email);
		await user.type(email, 'root@example.com');
		await user.type(dialog().getByLabelText(/^Current password/), 'root-password');
		await user.click(submit());

		await waitFor(() =>
			expect(mocks.send).toHaveBeenCalledWith('change_email', {
				email: 'root@example.com',
				current_password: 'root-password'
			})
		);
		expect(mocks.invalidate).toHaveBeenCalledWith('app:session');
	});

	it('sends nothing while the email is unchanged', async () => {
		const user = userEvent.setup();
		renderAccount({ email: 'root@example.com' });

		await openEmail(user);
		await user.type(dialog().getByLabelText(/^Current password/), 'root-password');
		await user.click(submit());

		expect(mocks.send).not.toHaveBeenCalled();
	});

	it('shows a refusal at its field and stays open', async () => {
		mocks.send.mockRejectedValue(
			new ApiError(400, 'invalid', {
				fields: { current_password: 'The current password is wrong.' }
			})
		);
		const user = userEvent.setup();
		renderAccount({ email: 'root@example.com' });

		await openEmail(user);
		await user.clear(dialog().getByLabelText(/^Email/));
		await user.type(dialog().getByLabelText(/^Email/), 'ops@example.com');
		await user.type(dialog().getByLabelText(/^Current password/), 'wrong-password');
		await user.click(submit());

		expect(await screen.findByText('The current password is wrong.')).toBeTruthy();
		expect(screen.getByRole('dialog')).toBeTruthy();
	});

	it('is not offered with a temporary password, whose form is on the page', () => {
		renderAccount({ email: 'root@example.com', must_change_password: true });

		expect(screen.queryByRole('button', { name: /^Change email/ })).toBeNull();
		expect(screen.getByLabelText(/^New password/)).toBeTruthy();
	});
});
