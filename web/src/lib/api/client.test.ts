import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isHttpError } from '@sveltejs/kit';
import { ApiError, errorMessage, get, listAll, pageError, post, query, send } from './client.ts';

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

		await get('list_issues', { project: ['backend', 'frontend'] });

		expect(fetchMock).toHaveBeenCalledWith('/api/ui/list_issues?project=backend&project=frontend', {
			credentials: 'same-origin'
		});
	});

	it('sends a change as JSON, without a query', async () => {
		fetchMock.mockResolvedValue(answer(200, { issues: [] }));

		await post('apply_issue_action', { issue_ids: ['k3x9q2mf'], action: 'resolve' });

		expect(fetchMock).toHaveBeenCalledWith('/api/ui/apply_issue_action', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: '{"issue_ids":["k3x9q2mf"],"action":"resolve"}'
		});
	});

	it('reports a refusal with the server message, without leaving the page', async () => {
		fetchMock.mockResolvedValue(answer(403, { error: 'cross-site request rejected' }));

		await expect(get('list_projects')).rejects.toEqual(
			new ApiError(403, 'cross-site request rejected')
		);
		expect(assign).not.toHaveBeenCalled();
	});

	it('names the refused fields and the record that already exists', async () => {
		const body = {
			error: 'name: taken',
			fields: { name: 'taken' },
			duplicate: { id: 'c0nnect1', name: 'alerts' }
		};

		fetchMock.mockResolvedValue(answer(400, body));

		const failure = await post('create_project', { name: 'backend' }).catch(
			(caught: unknown) => caught
		);

		expect(failure).toBeInstanceOf(ApiError);
		expect(failure).toMatchObject({
			status: 400,
			message: 'name: taken',
			fields: body.fields,
			duplicate: body.duplicate
		});
	});

	it('sends a plain request on its method, and an empty answer is nothing', async () => {
		fetchMock.mockResolvedValue(answer(200, { app_name: 'Exceptic', disabled: false }));

		expect(await send('login_state', { next: '/logs' })).toEqual({
			app_name: 'Exceptic',
			disabled: false
		});
		expect(fetchMock).toHaveBeenLastCalledWith('/api/ui/login_state?next=%2Flogs', {
			credentials: 'same-origin'
		});

		fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

		expect(await send('logout')).toBeUndefined();
		expect(fetchMock).toHaveBeenLastCalledWith('/api/ui/logout', {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: '{}'
		});
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

		try {
			pageError(new ApiError(403, 'issue k3x9q2mf is not in your projects'));
		} catch (caught) {
			thrown = caught;
		}

		expect(isHttpError(thrown, 403) && thrown.body.message).toBe(
			'issue k3x9q2mf is not in your projects'
		);

		const other = new Error('network down');

		expect(() => pageError(other)).toThrow(other);
	});
});

describe('no answer', () => {
	it('is a failure that says so, shown as an error page', async () => {
		fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));

		const failure = await get('count_logs', { query: 'rare' }).catch((caught: unknown) => caught);

		expect(failure).toBeInstanceOf(ApiError);
		expect(errorMessage(failure)).toBe('The server did not answer. Try again.');

		let thrown: unknown;

		try {
			pageError(failure);
		} catch (caught) {
			thrown = caught;
		}

		expect(isHttpError(thrown, 503) && thrown.body.message).toBe(
			'The server did not answer. Try again.'
		);
	});
});

describe('an answer without the server', () => {
	it('is no answer: a proxy that lost the server answers 5xx without a JSON error', async () => {
		fetchMock.mockResolvedValueOnce(
			new Response('', { status: 500, statusText: 'Internal Server Error' })
		);

		const empty = await get('count_logs').catch((caught: unknown) => caught);

		expect(empty).toBeInstanceOf(ApiError);
		expect(errorMessage(empty)).toBe('The server did not answer. Try again.');

		fetchMock.mockResolvedValueOnce(
			new Response('<html>Bad Gateway</html>', {
				status: 502,
				statusText: 'Bad Gateway',
				headers: { 'Content-Type': 'text/html' }
			})
		);

		const gateway = await get('count_logs').catch((caught: unknown) => caught);

		expect(errorMessage(gateway)).toBe('The server did not answer. Try again.');

		fetchMock.mockResolvedValueOnce(
			new Response(JSON.stringify({ error: 'internal error' }), { status: 500 })
		);

		expect(errorMessage(await get('count_logs').catch((caught: unknown) => caught))).toBe(
			'internal error'
		);
	});
});

describe('types', () => {
	it('keep reads and changes apart', () => {
		// Never run: the checks are the type errors expected below.
		const unused = () => {
			// @ts-expect-error a change is not a read (loads run on hover)
			void get('apply_issue_action', { issue_ids: ['k3x9q2mf'], action: 'resolve' });
			// @ts-expect-error a read is not a change
			void post('list_projects');
			// @ts-expect-error list_users gives every row at once: it takes no limit
			void listAll('list_users');
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

	it('is not a wrong password at the login, which the form shows', async () => {
		fetchMock.mockImplementation(() =>
			Promise.resolve(answer(401, { error: 'Wrong username or password.' }))
		);

		const page = await loadPage();

		await expect(
			page.send('login', { username: 'root', password: 'wrong-password' })
		).rejects.toEqual(new page.ApiError(401, 'Wrong username or password.'));
		expect(assign).not.toHaveBeenCalled();

		// Any other plain request: the session ended.
		void page.send('personal_token');

		await vi.waitFor(() => expect(assign).toHaveBeenCalledOnce());
	});
});

describe('errorMessage', () => {
	it("says the server's reason, or the fallback for anything else", () => {
		expect(errorMessage(new ApiError(400, 'name: taken'))).toBe('name: taken');
		expect(errorMessage(new TypeError('fetch failed'))).toBe('Something went wrong.');
		expect(errorMessage(new Error('x'), 'Could not load more events.')).toBe(
			'Could not load more events.'
		);
	});
});
