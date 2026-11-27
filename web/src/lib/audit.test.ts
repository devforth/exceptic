import { describe, expect, it, vi } from 'vitest';
import {
	actionTone,
	changedParts,
	diffRows,
	entryMessage,
	entryTitle,
	groupFields,
	itemValue,
	recordHref,
	searchParts,
	valueText,
	viaText
} from './audit.ts';

vi.mock('$app/paths', () => ({
	resolve: (route: string, params?: { id?: string | number }) =>
		route.replace('/(app)', '').replace(/\[id(=\w+)?\]/, String(params?.id ?? ''))
}));

describe('audit', () => {
	it('tones a create green, an edit blue, a delete red and the rest plain', () => {
		expect(actionTone('create')).toBe('success');
		expect(actionTone('edit')).toBe('info');
		expect(actionTone('delete')).toBe('error');
		expect(actionTone('pause')).toBe('plain');
	});

	it('titles an entry by its action and record, several records by their count', () => {
		expect(entryTitle({ action: 'edit', resource: 'issue' })).toBe('Edit issue');
		expect(entryTitle({ action: 'reset_key', resource: 'project' })).toBe('Reset DSN project');
		expect(
			entryTitle({ action: 'delete', resource: 'log', item_count: 3, record_name: '3 logs' })
		).toBe('Delete 3 logs');
	});

	it('says how the change came, with the agent that acted for the user', () => {
		expect(viaText({ source: 'ui' })).toBe('Web app');
		expect(viaText({ source: 'mcp', agent: 'Claude Code' })).toBe('MCP · Claude Code');
	});

	it('writes values as text', () => {
		expect(valueText('error')).toBe('error');
		expect(valueText(['backend', 'frontend'])).toBe('backend, frontend');
		expect(valueText([])).toBe('—');
		expect(valueText(null)).toBe('—');
		expect(valueText(false)).toBe('false');
		expect(valueText(25)).toBe('25');
	});

	it('lists each changed field once, by name, with the sides it has', () => {
		expect(
			diffRows({ status: 'unresolved' }, { status: 'resolved', ignored_until_events: 10 })
		).toEqual([
			{ field: 'ignored until events', after: '10' },
			{ field: 'status', before: 'unresolved', after: 'resolved' }
		]);
		expect(diffRows(null, { name: 'backend' })).toEqual([{ field: 'name', after: 'backend' }]);
		expect(diffRows({ name: 'backend' }, null)).toEqual([{ field: 'name', before: 'backend' }]);
		expect(diffRows(null, null)).toEqual([]);
	});

	it('links only a record that exists, a project to its settings for an admin', () => {
		const project = { resource: 'project', record_id: '7', exists: true };

		expect(recordHref(project, true)).toBe('/projects/7');
		expect(recordHref(project, false)).toBe('/projects/7/setup');
		expect(recordHref({ ...project, exists: false }, true)).toBeUndefined();
		expect(recordHref({ resource: 'issue', record_id: 'k3x9q2mf', exists: true }, false)).toBe(
			'/issues/k3x9q2mf'
		);
		expect(recordHref({ resource: 'log', record_id: '12', exists: true }, true)).toBeUndefined();
	});
});

describe('groupFields', () => {
	it('shows what every record changed alike once and the rest as columns', () => {
		const grouped = groupFields([
			{ record_name: 'TypeError', before: { status: 'unresolved' }, after: { status: 'resolved' } },
			{ record_name: 'ValueError', before: { status: 'ignored' }, after: { status: 'resolved' } }
		]);

		expect(grouped.common).toEqual({ before: {}, after: { status: 'resolved' } });
		expect(grouped.varying).toEqual([{ side: 'before', name: 'status', label: 'status before' }]);
	});

	it('leaves out a field that repeats the record name, and names a column without its side when there is one', () => {
		const grouped = groupFields([
			{ record_name: 'TypeError', before: { title: 'TypeError', level: 'error', events: 1 } },
			{ record_name: 'ValueError', before: { title: 'ValueError', level: 'warning', events: 1 } }
		]);

		expect(grouped.common).toEqual({ before: { events: 1 }, after: {} });
		expect(grouped.varying).toEqual([{ side: 'before', name: 'level', label: 'level' }]);
	});

	it("leaves out an ignore's bound, which its status already says", () => {
		const grouped = groupFields([
			{ record_name: 'TypeError', after: { status: 'ignored', ignored_until_events: 10 } },
			{ record_name: 'ValueError', after: { status: 'ignored', ignored_until_events: 100 } }
		]);

		expect(grouped.common).toEqual({ before: {}, after: { status: 'ignored' } });
		expect(grouped.varying).toEqual([]);
	});

	it("gives a record's value of a varying field as text, a dash where it has none", () => {
		const field = { side: 'before', name: 'status', label: 'status before' } as const;

		expect(itemValue({ before: { status: 'unresolved' } }, field)).toBe('unresolved');
		expect(itemValue({ after: { status: 'resolved' } }, field)).toBe('—');
	});
});

describe('changedParts', () => {
	it('marks what changed between what both values start and end with', () => {
		expect(changedParts('http://1d0f@localhost:8000/1', 'http://5036@localhost:8000/1')).toEqual({
			before: [
				{ text: 'http://', changed: false },
				{ text: '1d0f', changed: true },
				{ text: '@localhost:8000/1', changed: false }
			],
			after: [
				{ text: 'http://', changed: false },
				{ text: '5036', changed: true },
				{ text: '@localhost:8000/1', changed: false }
			]
		});
	});

	it('marks whole words, not the letters they differ in', () => {
		expect(changedParts('resolved', 'ignored')).toEqual({
			before: [{ text: 'resolved', changed: true }],
			after: [{ text: 'ignored', changed: true }]
		});
		expect(changedParts('fatal, error', 'fatal, warning')).toEqual({
			before: [
				{ text: 'fatal, ', changed: false },
				{ text: 'error', changed: true }
			],
			after: [
				{ text: 'fatal, ', changed: false },
				{ text: 'warning', changed: true }
			]
		});
	});

	it('keeps a value without the other side whole', () => {
		expect(changedParts(undefined, 'backend')).toEqual({
			before: [],
			after: [{ text: 'backend', changed: false }]
		});
	});
});

describe('searchParts', () => {
	it('marks every match of the search in any case', () => {
		expect(searchParts('ManyError backend Backend', 'BACKEND')).toEqual([
			{ text: 'ManyError ', changed: false },
			{ text: 'backend', changed: true },
			{ text: ' ', changed: false },
			{ text: 'Backend', changed: true }
		]);
		expect(searchParts('ManyError', '')).toEqual([{ text: 'ManyError', changed: false }]);
	});
});

describe('entryMessage', () => {
	it('says what an entry did in words around the record name', () => {
		expect(
			entryMessage({ action: 'edit', resource: 'issue', after: { status: 'resolved' } })
		).toEqual({
			lead: 'Resolved'
		});
		expect(
			entryMessage({
				action: 'edit',
				resource: 'issue',
				after: { status: 'ignored', ignored_until_events: 100 }
			})
		).toEqual({ lead: 'Ignored', tail: 'for 100 more events' });
		expect(
			entryMessage({
				action: 'create',
				resource: 'connection',
				after: { destination: 'ops@example.com' }
			})
		).toEqual({ lead: 'Created', tail: '→ ops@example.com' });
		expect(entryMessage({ action: 'create', resource: 'user', after: { role: 'user' } })).toEqual({
			lead: 'Added',
			tail: 'as user'
		});
		expect(
			entryMessage({
				action: 'edit',
				resource: 'project',
				before: { name: 'api', color: '#111111' },
				after: { name: 'backend', color: '#222222' }
			})
		).toEqual({ lead: 'Changed color, name of' });
		expect(entryMessage({ action: 'confirm', resource: 'connection' })).toEqual({
			lead: 'Confirmed'
		});
		expect(entryMessage({ action: 'reset_key', resource: 'project' })).toEqual({
			lead: 'Reset the DSN of'
		});
		expect(entryMessage({ action: 'delete', resource: 'session' })).toEqual({
			lead: 'Ended a session of'
		});
	});
});
