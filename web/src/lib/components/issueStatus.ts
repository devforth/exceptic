// How an issue status looks (StatusBadge.svelte, the issue list's rows): its tone, its sign
// and what it means, from the server's generated issueStatuses.
import { issueStatuses } from '#lib/api/catalog.gen.ts';
import { clockTime, countOf, shortDate } from '#lib/format.ts';
import type { RowTone } from './table.ts';

export type IssueStatus = (typeof issueStatuses)[number]['name'];

/** The color of a status's text; plain for "all". */
export type StatusTextTone = 'error' | 'success' | 'muted' | 'plain';

/** smallMark: a sign drawn larger than the others (the dot), shown smaller to match. */
type Look = { tone: StatusTextTone; mark: string; smallMark?: boolean; row: RowTone };

const looks: Record<IssueStatus, Look> = {
	unresolved: { tone: 'error', mark: '●', smallMark: true, row: 'error' },
	resolved: { tone: 'success', mark: '✓', row: 'success' },
	ignored: { tone: 'muted', mark: '⊘', row: 'faded' }
};

function known(name: string): name is IssueStatus {
	return name in looks;
}

/** "Unresolved", "All". */
export function statusLabel(name: string): string {
	return name.charAt(0).toUpperCase() + name.slice(1);
}

/** "Unresolved: still happening or not looked at yet"; nothing for "all". */
export function statusTitle(name: string): string | undefined {
	const status = issueStatuses.find((candidate) => candidate.name === name);

	return status && `${statusLabel(name)}: ${status.meaning}`;
}

/** The look of a status; "all" is plain, without a sign. */
export function statusLook(name: string): {
	tone: StatusTextTone;
	mark?: string;
	smallMark?: boolean;
} {
	return known(name) ? looks[name] : { tone: 'plain' };
}

/** The tint of a row of an issue with this status. */
export function statusRowTone(name: string): RowTone | undefined {
	return known(name) ? looks[name].row : undefined;
}

/** What ends an ignore, as the server sends it on an issue. */
export type IgnoreBounds = { ignored_until?: string | null; ignore_events_left?: number };

/** "until 25 Nov 14:00 UTC", "for 37 more events", both joined by "or"; nothing for good. */
export function ignoreBoundText(issue: IgnoreBounds): string | undefined {
	const parts = [];

	if (issue.ignored_until) {
		parts.push(
			`until ${shortDate(issue.ignored_until)} ${clockTime(issue.ignored_until).slice(0, 5)} UTC`
		);
	}

	if (issue.ignore_events_left) {
		const left = issue.ignore_events_left;

		parts.push(`for ${countOf(left, 'more events')}`);
	}

	return parts.length > 0 ? parts.join(' or ') : undefined;
}

/** The tooltip of an issue's status: "Ignored until 25 Nov 14:00 UTC", else what it means. */
export function issueStatusTitle(issue: IgnoreBounds & { status: string }): string | undefined {
	const bound = issue.status === 'ignored' ? ignoreBoundText(issue) : undefined;

	return bound ? `${statusLabel(issue.status)} ${bound}` : statusTitle(issue.status);
}
