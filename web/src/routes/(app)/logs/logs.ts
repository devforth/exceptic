// The logs page: its list after the count, a search that ran out of time, and deleting the
// checked logs (admins only; the server checks it too).
import { post, type In, type Out } from '#lib/api/client.ts';
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

type LogCount = Out<'count_logs'>;

/**
 * The page of the list after its count, searched no further back than the count searched
 * (from_id); none when nothing matched. A whole page: logs stored since the count push older
 * matches down, and from_id bounds the list anyway.
 */
export function listPage(
	count: LogCount,
	page: number,
	perPage: number
): Pick<In<'list_logs'>, 'limit' | 'offset' | 'from_id'> | undefined {
	if (count.count === 0) return undefined;

	return { limit: perPage, offset: (page - 1) * perPage, from_id: count.from_id };
}

/** Why the list shows fewer rows than its count, when its own search ran out of time. */
export function stoppedList(
	list: Pick<Out<'list_logs'>, 'stopped'> | undefined
): string | undefined {
	return list?.stopped
		? 'This page stopped before it read all its logs. Reload it, or narrow the search by date, project or level.'
		: undefined;
}

/** After "The search stopped after 10 s at the logs received <time>:". */
export function stoppedSearch(count: Pick<LogCount, 'count'>): string {
	const found = count.count === 0 ? 'No log' : count.count === 1 ? '1 log' : `${count.count} logs`;

	return `${found} found in the newer ones. Narrow the search by date, project or level to search further.`;
}
