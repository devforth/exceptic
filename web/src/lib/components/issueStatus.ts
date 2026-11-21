// How an issue status looks (StatusBadge.svelte, the issue list's rows): its tone, its sign
// and what it means, from the server's generated issueStatuses.
import { issueStatuses } from '#lib/api/catalog.gen.ts';
import type { BadgeTone } from './badge.ts';
import type { RowTone } from './table.ts';

export type IssueStatus = (typeof issueStatuses)[number]['name'];

type Look = { tone: BadgeTone; mark: string; row: RowTone };

const looks: Record<IssueStatus, Look> = {
	unresolved: { tone: 'error', mark: '●', row: 'error' },
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

/** The badge of a status; "all" is a plain one without a sign. */
export function statusLook(name: string): { tone: BadgeTone; mark?: string } {
	return known(name) ? looks[name] : { tone: 'plain' };
}

/** The tint of a row of an issue with this status. */
export function statusRowTone(name: string): RowTone | undefined {
	return known(name) ? looks[name].row : undefined;
}
