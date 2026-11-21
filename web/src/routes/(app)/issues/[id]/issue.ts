// The rules of the issue page that are not drawing: what a stack frame shows, who changed a
// status, what is said of an event that is not shown.
import type { Out } from '#lib/api/client.ts';

export type IssueDetails = Out<'get_issue_details'>;
export type EventDetails = NonNullable<IssueDetails['event']>;
export type Exception = NonNullable<EventDetails['exceptions']>[number];
export type Frame = NonNullable<Exception['frames']>[number];
export type StatusChange = NonNullable<IssueDetails['history']>[number];

/** "file.js:12:5", the file or the module; the line and column only when known. */
export function frameLocation(frame: Frame): string {
	return (
		(frame.location ?? '') +
		(frame.line ? `:${frame.line}` : '') +
		(frame.column ? `:${frame.column}` : '')
	);
}

/** The function of a frame, or <anonymous>. */
export function frameFunction(frame: Frame): string {
	return frame.function || '<anonymous>';
}

/** A frame unfolds only with something to show: browser SDKs send no source. */
export function frameUnfolds(frame: Frame): boolean {
	return (frame.source_lines?.length ?? 0) > 0 || Boolean(frame.vars);
}

/** The application's own frame that threw starts open. */
export function frameStartsOpen(frame: Frame): boolean {
	return Boolean(frame.in_app) && (frame.source_lines ?? []).some((line) => line.current);
}

/** Who made a status change: a user (via MCP), or the event that reopened it. */
export function changeAuthor(change: StatusChange): string {
	if (change.by_event) return 'reopened: happened again';

	return `by ${change.name || 'a deleted user'}${change.via_mcp ? ' via MCP' : ''}`;
}

/** What is said instead of an event that is not shown; nothing when it is. */
export function eventNotice(state: IssueDetails['event_state']): string | undefined {
	switch (state) {
		case 'unreadable':
			return "This event's stored data cannot be read.";
		case 'missing':
			return 'This event is no longer stored.';
		case 'none':
			return 'No events of this issue are stored.';
		default:
			return undefined;
	}
}

/** The SDK's word for handled, when it says. */
export function handledLabel(handled: boolean | null | undefined): string | undefined {
	if (handled === undefined || handled === null) return undefined;

	return handled ? 'handled' : 'unhandled';
}
