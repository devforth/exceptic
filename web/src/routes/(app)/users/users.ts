// What the users list and a user's page share: the questions before a reset and a delete,
// and the delete itself.
import { post } from '#lib/api/client.ts';
import type { Confirmation } from '#lib/components/confirm.ts';
import type { Role } from '#lib/components/role.ts';
import { toast } from '#lib/toast.svelte.ts';

/** The fields of UserForm. */
export type UserInput = { username: string; name: string; role: Role };

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

export async function deleteUser(id: string): Promise<void> {
	await post('delete_user', { user_id: id });
	toast.show('User deleted.');
}
