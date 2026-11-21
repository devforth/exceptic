import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	deleteConfirmation,
	deleteProject,
	paletteName,
	projectInput,
	usageSegments
} from './projects.ts';

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => vi.stubGlobal('fetch', fetchMock));

afterEach(() => {
	vi.unstubAllGlobals();
	fetchMock.mockReset();
});

describe('deleting a project', () => {
	it('names the project and what goes with it', () => {
		expect(deleteConfirmation('mobile')).toEqual({
			title: 'Delete project',
			message:
				'Delete the project mobile with all its issues, events and logs? Its DSN stops working. This cannot be undone.',
			confirmLabel: 'Delete'
		});
	});

	it('sends its ID', async () => {
		fetchMock.mockResolvedValue(new Response('{"done":"deleted project mobile"}', { status: 200 }));

		await deleteProject(12345678);

		const [url, init] = fetchMock.mock.calls[0];

		expect(url).toBe('/api/ui/delete_project');
		expect(JSON.parse(init?.body as string)).toEqual({ project: '12345678' });
	});
});

describe('projectInput', () => {
	const values = { name: ' mobile ', color: '#123456', eventLevels: [], logLevels: ['error'] };

	it('sends the whole form, empty level lists included', () => {
		expect(projectInput(values, false)).toEqual({
			name: 'mobile',
			color: '#123456',
			event_levels: [],
			log_levels: ['error']
		});
	});

	it('leaves the name of a project from PROJECTS out', () => {
		expect(projectInput(values, true)).not.toHaveProperty('name');
	});
});

describe('paletteName', () => {
	it('names the generated palette colors only', () => {
		expect(paletteName('#2563EB')).toBe('Blue');
		expect(paletteName('#123456')).toBeUndefined();
	});
});

describe('usageSegments', () => {
	const storage = {
		used_bytes: 400,
		used: '400 B',
		limit_bytes: 1000,
		limit: '1000 B',
		cleanup_stop_percent: 80,
		projects: [
			{ id: 1, name: 'backend', bytes: 300, size: '300 B' },
			{ id: 2, name: 'frontend', bytes: 0, size: '0 B' }
		],
		other_bytes: 100,
		other: '100 B'
	};

	it('splits the limit into the projects with data and the rest', () => {
		expect(usageSegments(storage, (name) => (name === 'backend' ? '#2563eb' : undefined))).toEqual([
			{ label: 'backend: 300 B', percent: 30, color: '#2563eb' },
			{ label: 'Other: 100 B', percent: 10 }
		]);
	});

	it('splits the size without a limit, and nothing while counting', () => {
		const unlimited = { ...storage, limit_bytes: undefined };

		expect(usageSegments(unlimited, () => undefined).map((segment) => segment.percent)).toEqual([
			75, 25
		]);
		expect(usageSegments({ ...storage, counting: true }, () => undefined)).toEqual([]);
	});
});
