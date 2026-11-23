// What the users list and a user's page share: the questions before a reset and a delete,
// and the delete itself.
import { invalidateAll } from '$app/navigation';
import { ApiError, post, type Out } from '#lib/api/client.ts';
import type { Confirmation } from '#lib/components/confirm.ts';
import type { ProjectsInput } from '#lib/components/projectsChoice.ts';
import type { Role } from '#lib/components/role.ts';
import { toast } from '#lib/toast.svelte.ts';

/** The fields of UserForm; no access when the viewer may not change it. */
export type UserInput = { username: string; name: string; role: Role; access?: ProjectsInput };

export function resetConfirmation(username: string): Confirmation {
	return {
		title: 'Reset password',
		message: `Give ${username} a temporary password? They are logged out and their coding agents are disconnected.`,
		confirmLabel: 'Reset password',
		tone: 'primary'
	};
}

export function deleteConfirmation(username: string): Confirmation {
	return {
		title: 'Delete user',
		message: `Delete the user ${username}? They are logged out and their coding agents are disconnected; their name stays in issue histories.`,
		confirmLabel: 'Delete'
	};
}

/** Why the viewer may not change a user they do not manage (another admin, the superadmin). */
export const roleLocked = 'Your role cannot change this user.';

type Managed = Pick<Out<'get_user'>, 'can_manage' | 'can_take_over' | 'take_over_reason'>;

/** Why the viewer may not rename, delete or reset the password of a user; undefined when they may. */
export function takeOverLock({ can_manage, can_take_over, take_over_reason }: Managed) {
	if (!can_manage) {
		return roleLocked;
	}

	return can_take_over ? undefined : take_over_reason;
}

export async function deleteUser(id: string): Promise<void> {
	await post('delete_user', { user_id: id });
	toast.show('User deleted.');
}

/**
 * Runs an action on a user and, when the server refuses it (rights changed or the user went
 * since the page loaded), reloads the page so its buttons show what is allowed now; the
 * refusal still reaches the dialog or form.
 */
export async function refreshingOnRefusal<Result>(action: () => Promise<Result>): Promise<Result> {
	try {
		return await action();
	} catch (failure) {
		if (failure instanceof ApiError && (failure.status === 403 || failure.status === 404)) {
			void invalidateAll();
		}

		throw failure;
	}
}
