import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import AccountPage from './+page.svelte';

vi.mock('$app/navigation', () => ({ invalidate: vi.fn() }));

function renderAccount(user: { email: string; must_change_password?: boolean }) {
	const session = { user: { display_name: 'Root', role: 'admin', ...user } };

	render(AccountPage, { data: { session } } as never);
}

describe('the account page', () => {
	it('shows the email but offers no way to change it: it is set once, when the user is added', () => {
		renderAccount({ email: 'root@example.com' });

		expect(screen.getByText(/root@example.com/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: /Change email/ })).toBeNull();
		expect(screen.getByRole('button', { name: /^Change password/ })).toBeTruthy();
	});

	it('shows only the password form with a temporary password', () => {
		renderAccount({ email: 'root@example.com', must_change_password: true });

		expect(screen.getByLabelText(/^New password/)).toBeTruthy();
		expect(screen.queryByRole('button', { name: /^Sessions and sign-ins/ })).toBeNull();
	});
});
