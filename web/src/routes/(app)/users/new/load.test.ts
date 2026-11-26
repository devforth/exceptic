import { describe, expect, it, vi } from 'vitest';
import { load } from './+page.ts';

vi.mock('#lib/api/client.ts', () => ({
	listAll: vi.fn(() => Promise.resolve([])),
	pageError: vi.fn()
}));

describe('the new user page', () => {
	it('offers the viewer their projects, whatever the role', async () => {
		await expect(load({} as never)).resolves.toEqual({ projects: [] });
	});
});
