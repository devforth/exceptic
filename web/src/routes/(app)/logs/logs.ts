// Deleting the checked logs on the logs page. Admins only; the server checks it too.
import { post, type In } from '#lib/api/client.ts';
import type { Confirmation } from '#lib/components/confirm.ts';
import { toast } from '#lib/toast.svelte.ts';

/** Why the checked logs cannot be deleted, if they cannot. */
export function selectionProblem(count: number): string | undefined {
	return count === 0 ? 'Select logs to delete them' : undefined;
}

/** The question before deleting one log or the checked ones. */
export function deleteConfirmation(count: number): Confirmation {
	return {
		title: count === 1 ? 'Delete log' : 'Delete logs',
		message: `Delete ${count === 1 ? 'this log' : `${count} selected logs`}? This cannot be undone.`,
		confirmLabel: 'Delete'
	};
}

/** What the toast says once done: "Log deleted.", "12 logs deleted.". */
export function deletedMessage(count: number): string {
	if (count === 0) return 'The logs were already deleted.';

	return count === 1 ? 'Log deleted.' : `${count} logs deleted.`;
}

/** Deletes logs and says how many went. */
export async function deleteLogs(input: In<'delete_logs'>): Promise<void> {
	const { deleted } = await post('delete_logs', input);

	toast.show(deletedMessage(deleted));
}
