import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isHttpError } from '@sveltejs/kit';
import { ApiError, get, listAll, pageError, post, query } from './client.ts';

const fetchMock = vi.fn<typeof fetch>();
const assign = vi.fn();
const storage = new Map<string, string>();

function answer(status: number, body: unknown, contentType = 'application/json') {
	const text = typeof body === 'string' ? body : JSON.stringify(body);

	return new Response(text, { status, headers: { 'Content-Type': contentType } });
}

beforeEach(() => {
	storage.clear();
	vi.stubGlobal('fetch', fetchMock);
	vi.stubGlobal('location', { pathname: '/projects', search: '?a=1', hash: '', assign });
	vi.stubGlobal('sessionStorage', {
		getItem: (key: string) => storage.get(key) ?? null,
		setItem: (key: string, value: string) => storage.set(key, value)
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
	fetchMock.mockReset();
	assign.mockReset();
});

describe('query', () => {
	it('repeats list names and leaves out empty values', () => {
		expect(
			query({ issue_ids: ['a', 'b'], status: 'all', cursor: undefined, project: null, limit: 5 })
		).toBe('?issue_ids=a&issue_ids=b&status=all&limit=5');
		expect(query({})).toBe('');
	});
});

describe('requests', () => {
	it('sends a read as a GET with its query', async () => {
		fetchMock.mockResolvedValue(answer(200, { issues: [] }));

		await get('get_issues', { issue_ids: ['k3x9q2mf', 'a7c2d4e9'] });

		expect(fetchMock).toHaveBeenCalledWith(
			'/api/ui/get_issues?issue_ids=k3x9q2mf&issue_ids=a7c2d4e9',
			{
				credentials: 'same-origin'
			}
		);
	});

	it('sends a change as JSON, without a query', async () => {
		fetchMock.mockResolvedValue(answer(200, { issues: [] }));

		await post('update_issue_status', { issue_ids: ['k3x9q2mf'], status: 'resolved' });

		expect(fetchMock).toHaveBeenCalledWith('/api/ui/update_issue_status', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: '{"issue_ids":["k3x9q2mf"],"status":"resolved"}'
		});
	});

	it('reads a text answer as text', async () => {
		fetchMock.mockResolvedValue(answer(200, 'the prompt', 'text/plain'));

		expect(await get('get_issue', { issue_id: 'k3x9q2mf' })).toBe('the prompt');
	});

	it('reports a refusal with the server message, without leaving the page', async () => {
		fetchMock.mockResolvedValue(answer(403, { error: 'cross-site request rejected' }));

		await expect(get('list_projects')).rejects.toEqual(
			new ApiError(403, 'cross-site request rejected')
		);
		expect(assign).not.toHaveBeenCalled();
	});

	it('walks every page of a list', async () => {
		fetchMock
			.mockResolvedValueOnce(answer(200, { items: [{ id: 1 }], next_cursor: '1' }))
			.mockResolvedValueOnce(answer(200, { items: [{ id: 2 }] }));

		expect(await listAll('list_projects')).toEqual([{ id: 1 }, { id: 2 }]);
		expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
			'/api/ui/list_projects?limit=100',
			'/api/ui/list_projects?limit=100&cursor=1'
		]);
	});
});

describe('pageError', () => {
	it('shows a refused request as the error page of its status', () => {
		let thrown: unknown;

		try {
			pageError(new ApiError(404, 'not found'));
		} catch (caught) {
			thrown = caught;
		}

		expect(isHttpError(thrown, 404) && thrown.body.message).toBe('not found');

		const other = new Error('network down');

		expect(() => pageError(other)).toThrow(other);
	});
});

describe('types', () => {
	it('keep reads and changes apart', () => {
		// Never run: the checks are the type errors expected below.
		const unused = () => {
			// @ts-expect-error a change is not a read (loads run on hover)
			void get('update_issue_status', { issue_ids: ['k3x9q2mf'], status: 'resolved' });
			// @ts-expect-error a read is not a change
			void post('list_projects');
		};

		expect(unused).toBeTypeOf('function');
	});
});

describe('an ended session', () => {
	// A page load: the client as the browser starts it.
	async function loadPage() {
		vi.resetModules();

		return await import('./client.ts');
	}

	it('sends the browser to the login and back, once for all the requests of a page', async () => {
		fetchMock.mockImplementation(() => Promise.resolve(answer(401, { error: 'login required' })));

		const page = await loadPage();
		let settled = 0;

		for (const pending of [page.get('get_session'), page.get('list_projects')]) {
			void pending.finally(() => settled++);
		}

		await vi.waitFor(() => expect(assign).toHaveBeenCalledOnce());
		await new Promise((resolve) => setTimeout(resolve, 10));

		expect(assign).toHaveBeenCalledWith('/login?next=%2Fprojects%3Fa%3D1');
		expect(settled).toBe(0);

		// Back from the login and still refused: an error, not another trip.
		const back = await loadPage();

		await expect(back.get('list_projects')).rejects.toEqual(
			new back.ApiError(401, 'login required')
		);
		expect(assign).toHaveBeenCalledOnce();
	});
});
