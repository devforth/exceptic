// The actions on issues (IssueActions.svelte): which an issue offers, the question each asks
// and what is said once done. One issue (its page) or the checked ones of the list.
import { choices, maxActionIssues } from '#lib/api/catalog.gen.ts';
import type { IconName } from '#lib/icons.ts';
import type { ButtonVariant } from './button.ts';
import type { Confirmation } from './confirm.ts';

export type IssueAction = (typeof choices.apply_issue_action.action)[number];

type IgnoreFor = (typeof choices.apply_issue_action.ignore_for)[number];

/** What ends an ignore: a time or a number of events more; neither for good. */
export type IgnoreBound = { ignore_for?: IgnoreFor; ignore_until_events?: number };

/** One way to ignore, an item of the Ignore menu. */
export type IgnoreOption = { label: string; icon: IconName; bound: IgnoreBound; ending: string };

const durations: Record<IgnoreFor, string> = {
	'1h': '1 hour',
	'24h': '24 hours',
	'7d': '7 days',
	'30d': '30 days'
};

/** The counts of events more the menu offers. */
const eventCounts = [10, 100, 1000];

/** Ignore for good, for each time the server takes, or until a number of events more. */
export const ignoreOptions: IgnoreOption[] = [
	{ label: 'Ignore forever', icon: 'forever', bound: {}, ending: '' },
	...choices.apply_issue_action.ignore_for.map((name) => ({
		label: `For ${durations[name]}`,
		icon: 'time' as const,
		bound: { ignore_for: name },
		ending: ` for ${durations[name]}`
	})),
	...eventCounts.map((count) => ({
		label: `Until ${count.toLocaleString('en-US')} more events`,
		icon: 'count' as const,
		bound: { ignore_until_events: count },
		ending: ` until it happens ${count.toLocaleString('en-US')} more times`
	}))
];

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

/**
 * The question before an action on one issue (count undefined) or on the checked ones; an
 * ignore says how long.
 */
export function confirmationOf(
	action: IssueAction,
	count?: number,
	ignore: IgnoreOption = ignoreOptions[0]
): Confirmation {
	const what = target(count);
	const until = ignore.ending ? ' until then' : '';
	const messages: Record<IssueAction, string> = {
		resolve: `Resolve ${what}? If the error happens again, it is reopened.`,
		ignore: `Ignore ${what}${ignore.ending}? New events are still recorded, but it stays out of the unresolved list${until}.`,
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
