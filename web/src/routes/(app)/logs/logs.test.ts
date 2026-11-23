import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { deleteConfirmation, deletedMessage, deleteLogs, selectionProblem } from './logs.ts';

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
