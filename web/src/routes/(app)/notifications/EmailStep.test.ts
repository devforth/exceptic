import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import EmailStep from './EmailStep.svelte';

const api = vi.hoisted(() => ({ post: vi.fn() }));

vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<typeof import('#lib/api/client.ts')>()),
	post: api.post
}));
vi.mock('$app/paths', () => ({
	resolve: (route: string, params?: { id?: string }) =>
		route.replace('/(app)', '').replace('[id=connectionId]', params?.id ?? '')
}));

describe('EmailStep', () => {
	it('goes from the address to the code to confirmed, Enter submitting each', async () => {
		const user = userEvent.setup();

		api.post
			.mockResolvedValueOnce({ email: 'ops@example.com', state: 'code', notice: 'We sent a code.' })
			.mockRejectedValueOnce(new ApiError(400, 'Wrong code.', { fields: { code: 'Wrong code.' } }))
			.mockResolvedValueOnce({
				email: 'ops@example.com',
				state: 'verified',
				notice: 'Address confirmed.',
				verified_at: '2026-11-20T10:00:00Z'
			});
		render(EmailStep, { typed: '', progress: { step: 'input', address: '', notice: '' } });

		await user.type(screen.getByLabelText(/^Email address/), 'Ops@Example.com{Enter}');
		await waitFor(() => expect(screen.getByText('We sent a code.')).toBeTruthy());
		expect(api.post).toHaveBeenLastCalledWith('start_email_verification', {
			email: 'Ops@Example.com'
		});

		await user.type(screen.getByLabelText(/^Confirmation code/), '000000{Enter}');
		await waitFor(() => expect(screen.getByText('Wrong code.')).toBeTruthy());

		await user.clear(screen.getByLabelText(/^Confirmation code/));
		await user.type(screen.getByLabelText(/^Confirmation code/), '123456{Enter}');
		await waitFor(() => expect(screen.getByText('Address confirmed.')).toBeTruthy());
		expect(api.post).toHaveBeenLastCalledWith('confirm_email', {
			email: 'ops@example.com',
			code: '123456'
		});

		await user.click(screen.getByRole('button', { name: 'Change email' }));
		expect(screen.getByLabelText(/^Email address/)).toBeTruthy();
	});

	it('links the connection that already has the address', async () => {
		const user = userEvent.setup();

		api.post.mockRejectedValueOnce(
			new ApiError(400, 'already sends', { duplicate: { id: 'abcd1234', name: 'Ops' } })
		);
		render(EmailStep, { typed: '', progress: { step: 'input', address: '', notice: '' } });
		await user.type(screen.getByLabelText(/^Email address/), 'ops@example.com{Enter}');

		await waitFor(() =>
			expect(screen.getByRole('link', { name: 'Ops' }).getAttribute('href')).toBe(
				'/notifications/abcd1234'
			)
		);
	});
});
