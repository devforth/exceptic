import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it, vi } from 'vitest';
import type { Out } from '#lib/api/client.ts';

const session = vi.hoisted(() => ({ temporary: false }));

vi.mock('#lib/api/client.ts', () => ({
	get: () =>
		Promise.resolve({
			user: { id: 'k3x9q2mf', must_change_password: session.temporary }
		} as Out<'get_session'>),
	pageError: (failure: unknown) => {
		throw failure;
	}
}));

const { load } = await import('./+layout.ts');

function loadOn(routeID: string) {
	return (load as (event: unknown) => Promise<unknown>)({
		depends: () => {},
		route: { id: routeID }
	});
}

describe('the app layout', () => {
	it('sends a temporary password to the account page, and only there', async () => {
		session.temporary = true;

		const refused = await loadOn('/(app)/logs').catch((failure: unknown) => failure);

		expect(isRedirect(refused) && refused.location).toBe('/account');
		await expect(loadOn('/(app)/account')).resolves.toBeTruthy();
	});

	it('opens every page with the user’s own password', async () => {
		session.temporary = false;

		await expect(loadOn('/(app)/logs')).resolves.toBeTruthy();
	});
});
