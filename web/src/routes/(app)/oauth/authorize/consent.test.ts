import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import Consent from './+page.svelte';

const app = vi.hoisted(() => ({ send: vi.fn(), assign: vi.fn() }));

vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<typeof import('#lib/api/client.ts')>()),
	send: app.send
}));

const session = { app_name: 'Exceptic', can_administer: false };
const request = {
	client_name: 'Codex <test>',
	client_site: 'agent.example',
	redirect_uri: 'http://127.0.0.1:44891/callback',
	form_token: 'form-token'
};
const query = 'client_id=x&state=s';

// The page's data: only the fields it reads.
const show = (data: { request?: typeof request; refused?: string }) =>
	render(Consent, { data: { session, query, ...data } } as never);

beforeEach(() => {
	vi.clearAllMocks();
	vi.stubGlobal('location', { assign: app.assign });
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('consent page', () => {
	it('names the agent, its site and where the browser returns, as text', () => {
		show({ request });

		expect(screen.getByText('Codex <test>')).toBeTruthy();
		expect(screen.getByText('agent.example')).toBeTruthy();
		expect(screen.getByText(/127\.0\.0\.1:44891\/callback/)).toBeTruthy();
		expect(screen.queryByText(/As you are an admin/)).toBeNull();
	});

	it('sends the decision with the form token and follows the answer', async () => {
		app.send.mockResolvedValue({ redirect_url: 'http://127.0.0.1:44891/callback?code=c' });
		show({ request });

		await userEvent.click(screen.getByRole('button', { name: 'Allow' }));

		expect(app.send).toHaveBeenCalledWith('authorization_decision', {
			query,
			form_token: 'form-token',
			allow: true
		});
		await waitFor(() =>
			expect(app.assign).toHaveBeenCalledWith('http://127.0.0.1:44891/callback?code=c')
		);
	});

	it('stays on a refused decision and says why', async () => {
		app.send.mockRejectedValue(new ApiError(403, 'invalid authorization form token'));
		show({ request });

		await userEvent.click(screen.getByRole('button', { name: 'Deny' }));

		expect(await screen.findByText('invalid authorization form token')).toBeTruthy();
		expect(app.assign).not.toHaveBeenCalled();
	});

	it('shows a refused request without buttons', () => {
		show({ refused: 'redirect_uri is not one the agent registered' });

		expect(screen.getByRole('alert').textContent).toContain('not one the agent registered');
		expect(screen.queryByRole('button')).toBeNull();
	});
});
