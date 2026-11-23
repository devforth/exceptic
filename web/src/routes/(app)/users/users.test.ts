import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import { refreshingOnRefusal, roleLocked, takeOverLock } from './users.ts';

const invalidateAll = vi.fn();

vi.mock('$app/navigation', () => ({
	invalidateAll: () => {
		invalidateAll();
	}
}));

afterEach(() => {
	invalidateAll.mockReset();
});

describe('refreshingOnRefusal', () => {
	it('reloads the page when the server refuses, then passes the refusal on', async () => {
		const refused = new ApiError(
			403,
			"Only an admin with all of this user's projects can change it."
		);

		await expect(refreshingOnRefusal(() => Promise.reject(refused))).rejects.toBe(refused);
		expect(invalidateAll).toHaveBeenCalledOnce();
	});

	it('reloads for a user gone meanwhile too, not for other failures or a success', async () => {
		await expect(
			refreshingOnRefusal(() => Promise.reject(new ApiError(404, 'not found')))
		).rejects.toThrow();
		expect(invalidateAll).toHaveBeenCalledOnce();

		await expect(
			refreshingOnRefusal(() => Promise.reject(new ApiError(500, 'internal error')))
		).rejects.toThrow();
		await expect(refreshingOnRefusal(() => Promise.resolve('done'))).resolves.toBe('done');
		expect(invalidateAll).toHaveBeenCalledOnce();
	});
});

describe('takeOverLock', () => {
	it('says why the viewer may not change a user, by the reason the server gave', () => {
		const reason = "Only an admin with all of this user's projects can change it.";

		expect(takeOverLock({ can_manage: true, can_take_over: true })).toBeUndefined();
		expect(takeOverLock({ can_manage: true, can_take_over: false, take_over_reason: reason })).toBe(
			reason
		);
	});

	it('names the role, not the superadmin, for a user the viewer does not manage', () => {
		// An admin's row of another admin: the superadmin is not the only one locked.
		expect(takeOverLock({ can_manage: false, can_take_over: false })).toBe(roleLocked);
		expect(roleLocked).not.toMatch(/superadmin/);
	});
});
