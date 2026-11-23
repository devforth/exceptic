import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { projectPalette } from '#lib/api/catalog.gen.ts';
import {
	deleteConfirmation,
	deleteProject,
	freeColor,
	keptFacts,
	errorsKept,
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
	it('sends the whole form, empty level lists included', () => {
		expect(projectInput({ color: '#123456', logLevels: [], diskShare: 40 })).toEqual({
			color: '#123456',
			log_levels: [],
			disk_share: 40
		});
	});
});

describe('freeColor', () => {
	it('takes the first palette color no project shows', () => {
		expect(freeColor([projectPalette[0].color.toUpperCase()], '#000000')).toBe(
			projectPalette[1].color
		);
	});

	it('falls back when every one is taken', () => {
		expect(
			freeColor(
				projectPalette.map((entry) => entry.color),
				'#123456'
			)
		).toBe('#123456');
	});
});

describe('keptFacts', () => {
	// Local times: days are calendar days of the viewer's time zone.
	const now = new Date(2026, 10, 23, 0, 10).getTime();

	it('counts calendar days back to the oldest record of each kind', () => {
		expect(
			keptFacts(
				{
					logs_below_error: new Date(2026, 10, 20, 23, 50).toISOString(),
					errors: new Date(2026, 10, 22, 0, 5).toISOString()
				},
				now
			)
		).toEqual([
			{ label: 'Logs below error', value: '3 days', detail: 'since 20 Nov' },
			{ label: 'Error logs', value: 'none stored' },
			{ label: 'Errors', value: '1 day', detail: 'since 22 Nov' }
		]);
	});

	it('says today for a record of the same day', () => {
		expect(keptFacts({ errors: new Date(2026, 10, 23, 0, 1).toISOString() }, now)[2]).toEqual({
			label: 'Errors',
			value: 'today',
			detail: 'since 23 Nov'
		});
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
			{ id: 1, name: 'backend', bytes: 300, size: '300 B', limit_bytes: 0, oldest: {} },
			{ id: 2, name: 'frontend', bytes: 0, size: '0 B', limit_bytes: 0, oldest: {} }
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

describe('errorsKept', () => {
	const now = new Date(2026, 10, 23, 12).getTime();

	it('says how far back the errors go, not the logs', () => {
		expect(
			errorsKept(
				{
					errors: new Date(2026, 10, 16, 9).toISOString(),
					logs_below_error: new Date(2026, 10, 10, 9).toISOString()
				},
				now
			)
		).toBe('errors kept 7 days');
		expect(errorsKept({ errors: new Date(2026, 10, 22, 9).toISOString() }, now)).toBe(
			'errors kept 1 day'
		);
		expect(errorsKept({ errors: new Date(2026, 10, 23, 1).toISOString() }, now)).toBe(
			'errors kept today'
		);
	});

	it('says nothing without errors', () => {
		expect(errorsKept({ error_logs: new Date(2026, 10, 20).toISOString() }, now)).toBeUndefined();
		expect(errorsKept(undefined, now)).toBeUndefined();
	});
});
