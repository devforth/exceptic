// The audit log: how its entries read (resource, action and its badge, the changed fields) and
// where a record's page is.
import { resolve } from '$app/paths';
import type { Item } from '#lib/api/client.ts';
import type { BadgeTone } from '#lib/components/badge.ts';

export type AuditEntry = Item<'list_audit'>;

/** The name of each kind of record. */
export const resourceLabels: Record<string, string> = {
	issue: 'Issue',
	log: 'Log',
	project: 'Project',
	connection: 'Notification',
	user: 'User',
	session: 'Session',
	agent: 'Coding agent'
};

/** What an action reads as; the ones not here read as their name. */
const actionLabels: Record<string, string> = {
	reset_key: 'reset DSN',
	reset_password: 'reset password',
	unlink_google: 'unlink Google'
};

export function resourceLabel(resource: string): string {
	return resourceLabels[resource] ?? resource;
}

export function actionLabel(action: string): string {
	return actionLabels[action] ?? action;
}

/** A create is green, an edit blue, a delete red; the other actions plain. */
export function actionTone(action: string): BadgeTone {
	switch (action) {
		case 'create':
			return 'success';
		case 'edit':
			return 'info';
		case 'delete':
			return 'error';
		default:
			return 'plain';
	}
}

/** What an entry did in words, around its record's name: "Resolved" TypeError…, "Created" ops "→ ops@example.com". */
export type EntryMessage = { lead: string; tail?: string };

/** The verb of an issue's new status. */
const statusVerbs: Record<string, string> = {
	resolved: 'Resolved',
	ignored: 'Ignored',
	unresolved: 'Reopened'
};

/** The fields an edit changed, by name, for "Changed name, color of". */
function changedNames(entry: Partial<Pick<AuditEntry, 'before' | 'after'>>): string[] {
	// all_projects and projects are one setting to a reader: "projects".
	const names = [...Object.keys(entry.before ?? {}), ...Object.keys(entry.after ?? {})].map(
		(name) => (name === 'all_projects' ? 'projects' : name).replaceAll('_', ' ')
	);

	return [...new Set(names)].sort();
}

/** An entry's message: a verb before the record's name and what it became after it. */
export function entryMessage(
	entry: Pick<AuditEntry, 'action' | 'resource'> & Partial<Pick<AuditEntry, 'before' | 'after'>>
): EntryMessage {
	const after = entry.after ?? {};
	const status = typeof after.status === 'string' ? statusVerbs[after.status] : undefined;

	switch (entry.action) {
		case 'create':
			if (entry.resource === 'connection' && after.destination) {
				return { lead: 'Created', tail: `→ ${valueText(after.destination)}` };
			}
			if (entry.resource === 'user' && after.role) {
				return { lead: 'Added', tail: `as ${valueText(after.role)}` };
			}

			return { lead: entry.resource === 'connection' ? 'Created' : 'Added' };
		case 'edit': {
			if (status) {
				const events = after.ignored_until_events;

				return events
					? { lead: status, tail: `for ${valueText(events)} more events` }
					: { lead: status };
			}

			const names = changedNames(entry);

			return { lead: names.length ? `Changed ${names.join(', ')} of` : 'Changed' };
		}
		case 'delete':
			if (entry.resource === 'session') return { lead: 'Ended a session of' };
			if (entry.resource === 'agent') return { lead: 'Disconnected' };

			return { lead: 'Deleted' };
		case 'pause':
			return { lead: 'Paused' };
		case 'resume':
			return { lead: 'Resumed' };
		case 'reset_key':
			return { lead: 'Reset the DSN of' };
		case 'reset_password':
			return { lead: 'Reset the password of' };
		case 'retry':
			return { lead: 'Retried the waiting messages of' };
		case 'confirm':
			return { lead: 'Confirmed' };
		case 'unlink_google':
			return { lead: 'Unlinked Google from' };
		default:
			return { lead: actionLabel(entry.action) };
	}
}

/** The title of an entry: its action and record, e.g. "Edit issue", "Delete 3 logs". */
export function entryTitle(
	entry: Pick<AuditEntry, 'action' | 'resource' | 'record_name' | 'item_count'>
): string {
	const action = actionLabel(entry.action);
	const what = entry.item_count ? entry.record_name : resourceLabel(entry.resource).toLowerCase();

	return `${action.charAt(0).toUpperCase()}${action.slice(1)} ${what}`;
}

/** The look of a value before (red) and after (green) a change, and of the part that changed. */
export const sideClasses = {
	before: { line: 'bg-error-bg text-error', changed: 'bg-error/25' },
	after: { line: 'bg-success-bg text-success', changed: 'bg-success/25' }
} as const;

/** A piece of a value: whether it is the part that changed. */
export type ValuePart = { text: string; changed: boolean };

/**
 * A value before and after a change in parts, as a diff marks words: what both start and end with,
 * and between them the part that changed, widened to whole words. A value without the other side is one plain part.
 */
export function changedParts(
	before: string | undefined,
	after: string | undefined
): { before: ValuePart[]; after: ValuePart[] } {
	if (before === undefined || after === undefined) {
		return {
			before: before === undefined ? [] : [{ text: before, changed: false }],
			after: after === undefined ? [] : [{ text: after, changed: false }]
		};
	}

	let start = 0;

	while (start < before.length && start < after.length && before[start] === after[start]) start++;

	let end = 0;

	while (
		end < before.length - start &&
		end < after.length - start &&
		before[before.length - 1 - end] === after[after.length - 1 - end]
	) {
		end++;
	}

	// Whole words are marked: a change that starts or ends inside a word takes all of it.
	const word = (character: string | undefined) => character !== undefined && /\w/.test(character);

	while (start > 0 && word(before[start - 1]) && (word(before[start]) || word(after[start])))
		start--;

	while (
		end > 0 &&
		word(before[before.length - end]) &&
		(word(before[before.length - end - 1]) || word(after[after.length - end - 1]))
	) {
		end--;
	}

	const parts = (text: string): ValuePart[] =>
		[
			{ text: text.slice(0, start), changed: false },
			{ text: text.slice(start, text.length - end), changed: true },
			{ text: text.slice(text.length - end), changed: false }
		].filter((part) => part.text !== '');

	return { before: parts(before), after: parts(after) };
}

type Fields = Record<string, unknown> | null | undefined;

/** A record of an entry whose request changed several. */
export type AuditItemFields = { record_name?: string; before?: Fields; after?: Fields };

/** A field that differs between the records: its side and the label of its column. */
export type ItemField = { side: 'before' | 'after'; name: string; label: string };

/** Fields too detailed for the list of a request's records: an ignore's bound (its status says ignored). */
const listDetails = ['ignored_until', 'ignored_until_events'];

/**
 * The fields of a request's records: those alike in every record once (`common`, as one change),
 * the others as a column each (`varying`, "status before" when the records have both sides). A
 * field that only repeats a record's name (an issue's title) or details another (`listDetails`) is
 * left out.
 */
export function groupFields(items: readonly AuditItemFields[]): {
	common: { before: Record<string, unknown>; after: Record<string, unknown> };
	varying: ItemField[];
} {
	const common = { before: {} as Record<string, unknown>, after: {} as Record<string, unknown> };
	const varying: ItemField[] = [];
	const sides = (['before', 'after'] as const).filter((side) =>
		items.some((item) => Object.keys(item[side] ?? {}).length)
	);

	for (const side of sides) {
		const names = [...new Set(items.flatMap((item) => Object.keys(item[side] ?? {})))].sort();

		for (const name of names) {
			const texts = items.map((item) =>
				item[side] && name in item[side] ? valueText(item[side][name]) : undefined
			);

			if (
				listDetails.includes(name) ||
				texts.every((text, index) => text === items[index].record_name)
			) {
				continue;
			}

			if (texts.every((text) => text !== undefined && text === texts[0])) {
				common[side][name] = items[0][side]?.[name];
			} else {
				const label = name.replaceAll('_', ' ');

				varying.push({ side, name, label: sides.length > 1 ? `${label} ${side}` : label });
			}
		}
	}

	return { common, varying };
}

/** A record's value of a varying field, as text. */
export function itemValue(item: AuditItemFields, field: ItemField): string {
	return valueText(item[field.side]?.[field.name]);
}

/**
 * Text in parts, those that match the search (any case) marked as changed: what the list's search
 * found in a record's name. One plain part without a match or a search.
 */
export function searchParts(text: string, query: string): ValuePart[] {
	const needle = query.trim().toLowerCase();

	if (!needle) return [{ text, changed: false }];

	const parts: ValuePart[] = [];
	const lower = text.toLowerCase();
	let from = 0;

	for (let at = lower.indexOf(needle); at >= 0; at = lower.indexOf(needle, from)) {
		if (at > from) parts.push({ text: text.slice(from, at), changed: false });
		parts.push({ text: text.slice(at, at + needle.length), changed: true });
		from = at + needle.length;
	}

	if (from < text.length) parts.push({ text: text.slice(from), changed: false });

	return parts;
}

/** How the change came. */
export const sourceLabels: Record<string, string> = { ui: 'Web app', mcp: 'MCP', api: 'REST API' };

/** How the change came, with the agent that acted for the user through MCP: "MCP · Claude Code". */
export function viaText(entry: Pick<AuditEntry, 'source' | 'agent'>): string {
	const source = sourceLabels[entry.source] ?? entry.source;

	return entry.agent ? `${source} · ${entry.agent}` : source;
}

/** One changed field: its value before and after, as text; undefined where there is none. */
export type DiffRow = { field: string; before?: string; after?: string };

/** A field's value as text: lists joined, nothing as a dash, anything else as JSON. */
export function valueText(value: unknown): string {
	if (value === null || value === undefined || value === '') return '—';
	if (typeof value === 'string') return value;
	if (Array.isArray(value)) {
		return value.length === 0 ? '—' : value.map((item) => valueText(item)).join(', ');
	}

	return JSON.stringify(value);
}

/** The fields of a change, by name: each that either side has. */
export function diffRows(
	before: Record<string, unknown> | null | undefined,
	after: Record<string, unknown> | null | undefined
): DiffRow[] {
	const fields = [...new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})])].sort();

	return fields.map((field) => ({
		field: field.replaceAll('_', ' '),
		...(before && field in before ? { before: valueText(before[field]) } : {}),
		...(after && field in after ? { after: valueText(after[field]) } : {})
	}));
}

/**
 * The page of an entry's record, when it still exists and the viewer may open it: a project's
 * settings for an admin, its setup for a user.
 */
export function recordHref(
	entry: Pick<AuditEntry, 'resource' | 'record_id' | 'exists'>,
	canAdminister: boolean
): string | undefined {
	if (!entry.exists) return undefined;

	const id = entry.record_id;

	switch (entry.resource) {
		case 'issue':
			return resolve('/(app)/issues/[id]', { id });
		case 'project':
			return canAdminister
				? resolve('/(app)/projects/[id=projectId]', { id: Number(id) })
				: resolve('/(app)/projects/[id=projectId]/setup', { id: Number(id) });
		case 'connection':
			return resolve('/(app)/notifications/[id=connectionId]', { id });
		case 'user':
			return resolve('/(app)/users/[id]', { id });
		default:
			return undefined;
	}
}
