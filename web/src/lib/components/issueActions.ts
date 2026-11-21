// The actions on issues (IssueActions.svelte): which an issue offers, the question each asks
// and what is said once done. One issue (its page) or the checked ones of the list.
import { choices, maxActionIssues } from '#lib/api/catalog.gen.ts';
import type { ButtonVariant } from './button.ts';
import type { Confirmation } from './confirm.ts';

export type IssueAction = (typeof choices.apply_issue_action.action)[number];

const labels: Record<IssueAction, string> = {
	resolve: 'Resolve',
	ignore: 'Ignore',
	reopen: 'Reopen',
	delete: 'Delete'
};

const done: Record<IssueAction, string> = {
	resolve: 'resolved',
	ignore: 'ignored',
	reopen: 'reopened',
	delete: 'deleted'
};

export function actionLabel(action: IssueAction): string {
	return labels[action];
}

export function actionVariant(action: IssueAction): ButtonVariant {
	return action === 'resolve' ? 'primary' : action === 'delete' ? 'danger' : 'default';
}

/**
 * The actions of an issue of this status: Resolve and Ignore while unresolved, Reopen
 * otherwise; checked issues (no status) get all of them. Delete only for who may delete.
 */
export function actionsFor(status: string | undefined, canDelete: boolean): IssueAction[] {
	const actions = choices.apply_issue_action.action.filter((action) => {
		if (action === 'delete') return canDelete;
		if (status === undefined) return true;

		return status === 'unresolved' ? action !== 'reopen' : action === 'reopen';
	});

	return [...actions];
}

/** "this issue", or "3 selected issues". */
function target(count: number | undefined): string {
	if (count === undefined) return 'this issue';

	return `${count} selected ${count === 1 ? 'issue' : 'issues'}`;
}

/** The question before an action on one issue (count undefined) or on the checked ones. */
export function confirmationOf(action: IssueAction, count?: number): Confirmation {
	const what = target(count);
	const messages: Record<IssueAction, string> = {
		resolve: `Resolve ${what}? If the error happens again, it is reopened.`,
		ignore: `Ignore ${what}? New events are still recorded, but it stays out of the unresolved list.`,
		reopen: `Reopen ${what}?`,
		delete: `Delete ${what} and all its events? This cannot be undone. If the error happens again, it starts a new issue.`
	};

	return {
		title: `${labels[action]} ${count === undefined || count === 1 ? 'issue' : 'issues'}`,
		message: messages[action],
		confirmLabel: labels[action],
		tone: action === 'delete' ? 'danger' : 'primary'
	};
}

/** What the toast says once done: "Issue resolved.", "3 issues ignored.". */
export function doneMessage(action: IssueAction, count: number): string {
	return count === 1 ? `Issue ${done[action]}.` : `${count} issues ${done[action]}.`;
}

/** Why the checked issues cannot be changed together, if they cannot. */
export function selectionProblem(count: number): string | undefined {
	if (count === 0) return 'Select issues to change them together';
	if (count > maxActionIssues) return `At most ${maxActionIssues} issues at once`;

	return undefined;
}
