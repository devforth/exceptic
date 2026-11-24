import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	deleteConfirmation,
	deletedMessage,
	deleteLogs,
	listPage,
	selectionProblem,
	stoppedList,
	stoppedSearch
} from './logs.ts';

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => vi.stubGlobal('fetch', fetchMock));

afterEach(() => {
	vi.unstubAllGlobals();
	fetchMock.mockReset();
});

describe('deleting logs', () => {
	it('needs a checked log', () => {
		expect(selectionProblem(0)).toBe('Select logs to delete them');
		expect(selectionProblem(3)).toBeUndefined();
	});

	it('asks about one log or the checked ones', () => {
		expect(deleteConfirmation(1).message).toBe('Delete this log? This cannot be undone.');
		expect(deleteConfirmation(3)).toMatchObject({
			title: 'Delete logs',
			message: 'Delete 3 selected logs? This cannot be undone.'
		});
	});

	it('says how many went', () => {
		expect(deletedMessage(1)).toBe('Log deleted.');
		expect(deletedMessage(12)).toBe('12 logs deleted.');
		expect(deletedMessage(0)).toBe('The logs were already deleted.');
	});

	it('sends the checked ids', async () => {
		fetchMock.mockResolvedValue(new Response('{"deleted":2}', { status: 200 }));

		await deleteLogs({ log_ids: [41, 42] });

		const [url, init] = fetchMock.mock.calls[0];

		expect(url).toBe('/api/ui/delete_logs');
		expect(JSON.parse(init?.body as string)).toEqual({ log_ids: [41, 42] });
	});
});

describe('the list after its count', () => {
	it('is not asked for when nothing matched', () => {
		expect(listPage({ count: 0 }, 1, 20)).toBeUndefined();
		expect(listPage({ count: 0, stopped: true, from_id: 40 }, 1, 20)).toBeUndefined();
	});

	it('asks for the rows the count found, no further back than it searched', () => {
		// A whole page: logs stored since the count push older matches down, and from_id
		// bounds the list anyway.
		expect(listPage({ count: 45, from_id: 700 }, 3, 20)).toEqual({
			limit: 20,
			offset: 40,
			from_id: 700
		});
		expect(listPage({ count: 10000, more: true, from_id: 9 }, 2, 20)).toEqual({
			limit: 20,
			offset: 20,
			from_id: 9
		});
		expect(listPage({ count: 3, stopped: true, from_id: 812 }, 1, 20)).toMatchObject({
			limit: 20,
			from_id: 812
		});
	});
});

describe('a list that stopped', () => {
	it('says so only when it stopped', () => {
		expect(stoppedList({})).toBeUndefined();
		expect(stoppedList(undefined)).toBeUndefined();
		expect(stoppedList({ stopped: true })).toContain('stopped before');
	});
});

describe('a search that stopped', () => {
	it('says what it found and how to search further', () => {
		expect(stoppedSearch({ count: 1 })).toBe(
			'1 log found in the newer ones. Narrow the search by date, project or level to search further.'
		);
		expect(stoppedSearch({ count: 0 })).toBe(
			'No log found in the newer ones. Narrow the search by date, project or level to search further.'
		);
		expect(stoppedSearch({ count: 7 })).toContain('7 logs found');
	});
});
