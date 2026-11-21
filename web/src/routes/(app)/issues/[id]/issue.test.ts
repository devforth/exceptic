import { describe, expect, it } from 'vitest';
import {
	changeAuthor,
	eventNotice,
	frameFunction,
	frameLocation,
	frameStartsOpen,
	frameUnfolds,
	handledLabel,
	type Frame,
	type StatusChange
} from './issue.ts';

const frame = (fields: Partial<Frame>): Frame => ({ source_lines: [], ...fields });
const change = (fields: Partial<StatusChange>): StatusChange => ({
	from: 'unresolved',
	to: 'resolved',
	created_at: '2026-11-20T12:00:00Z',
	...fields
});

describe('stack frames', () => {
	it('say where they are', () => {
		expect(frameLocation(frame({ location: '/app/a.js', line: 12, column: 5 }))).toBe(
			'/app/a.js:12:5'
		);
		expect(frameLocation(frame({ location: 'app.module' }))).toBe('app.module');
		expect(frameFunction(frame({}))).toBe('<anonymous>');
	});

	it('unfold only with source or variables, open when the app threw there', () => {
		const bare = frame({ location: '/app/bare.js', in_app: true });
		const source = frame({
			in_app: true,
			source_lines: [
				{ number: 1, text: 'a' },
				{ number: 2, text: 'throw error', current: true }
			]
		});

		expect(frameUnfolds(bare)).toBe(false);
		expect(frameUnfolds(frame({ vars: '{}' }))).toBe(true);
		expect(frameUnfolds(source)).toBe(true);
		expect(frameStartsOpen(source)).toBe(true);
		expect(frameStartsOpen({ ...source, in_app: false })).toBe(false);
	});

	it('say whether the SDK caught the error, when it says', () => {
		expect(handledLabel(true)).toBe('handled');
		expect(handledLabel(false)).toBe('unhandled');
		expect(handledLabel(null)).toBeUndefined();
	});
});

describe('the status history', () => {
	it('names who changed it', () => {
		expect(changeAuthor(change({ name: 'Dev One' }))).toBe('by Dev One');
		expect(changeAuthor(change({ name: 'admin', via_mcp: true }))).toBe('by admin via MCP');
		expect(changeAuthor(change({}))).toBe('by a deleted user');
		expect(changeAuthor(change({ by_event: true, event_id: 'abc' }))).toBe(
			'reopened: happened again'
		);
	});
});

describe('an event not shown', () => {
	it('is said why', () => {
		expect(eventNotice('shown')).toBeUndefined();
		expect(eventNotice('missing')).toBe('This event is no longer stored.');
		expect(eventNotice('none')).toBe('No events of this issue are stored.');
		expect(eventNotice('unreadable')).toBe("This event's stored data cannot be read.");
	});
});
