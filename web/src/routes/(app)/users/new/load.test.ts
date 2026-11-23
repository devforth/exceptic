import { isHttpError } from '@sveltejs/kit';
import { describe, expect, it, vi } from 'vitest';
import { load } from './+page.ts';

vi.mock('#lib/api/client.ts', () => ({
	listAll: vi.fn(() => Promise.resolve([])),
	pageError: vi.fn()
}));

type LoadEvent = Parameters<typeof load>[0];

function event(canAdminister: boolean): LoadEvent {
	return {
		parent: () => Promise.resolve({ session: { can_administer: canAdminister } })
	} as never;
}

describe('the new user page', () => {
	it('tells another role it may not add users (403), as the server does', async () => {
		const thrown = await Promise.resolve(load(event(false))).catch((failure: unknown) => failure);

		expect(isHttpError(thrown, 403)).toBe(true);
	});

	it('offers an admin their projects', async () => {
		await expect(load(event(true))).resolves.toEqual({ projects: [] });
	});
});
