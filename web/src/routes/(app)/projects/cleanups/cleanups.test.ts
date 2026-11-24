import { describe, expect, it } from 'vitest';
import { arrivedLine, compactionLine, compactionState, heldLine, type Run } from './cleanups.ts';

describe('the compaction after a run', () => {
	it('reads as its state', () => {
		expect(compactionState({ status: 'pending' })).toEqual({
			text: 'compacting…',
			tone: 'warning'
		});
		expect(
			compactionState({
				status: 'done',
				freed: '120 MB',
				took: '18s',
				files_before: '2 GB',
				files_after: '1.9 GB'
			})
		).toEqual({ text: '−120 MB in 18s', tone: 'success' });
		expect(compactionState({ status: 'skipped' }).text).toBe('not needed');
		expect(compactionState({ status: 'failed', error: 'disk full' }).tone).toBe('error');
	});

	it('says in the details what the files lost', () => {
		expect(
			compactionLine(
				{ status: 'done', took: '18s', files_before: '2 GB', files_after: '1.9 GB' },
				10,
				3
			)
		).toBe('Compacted afterwards in 18s: the files went from 2 GB to 1.9 GB.');
		expect(compactionLine({ status: 'skipped' }, 10, 3)).toContain('under 10%');
		expect(compactionLine({ status: 'failed', error: 'disk full' }, 10, 3)).toContain('disk full');
		expect(compactionLine(null, 10, 3)).toBe('');
	});

	it('says when data arriving meanwhile grew the files', () => {
		expect(
			compactionLine(
				{
					status: 'done',
					took: '40s',
					freed: '80 MB',
					files_before: '2.1 GB',
					files_after: '2.17 GB',
					files_grew: true
				},
				10,
				3
			)
		).toBe(
			'Compacted afterwards in 40s: it freed 80 MB, while data arriving meanwhile took the files from 2.1 GB to 2.17 GB.'
		);
	});

	it('says a failed one is tried again, then that it failed', () => {
		const retried = { status: 'pending', error: 'disk full', attempts: 1 };
		expect(compactionState(retried)).toEqual({ text: 'retrying…', tone: 'warning' });
		expect(compactionLine(retried, 10, 3)).toBe(
			'The compaction afterwards failed (disk full); it is tried again on the next pass, 1 of 3 attempts made.'
		);
		expect(compactionLine({ status: 'failed', error: 'disk full', attempts: 3 }, 10, 3)).toBe(
			'The compaction afterwards failed 3 times, the last: disk full.'
		);
	});
});

describe('the data that arrived during a run', () => {
	const run = {
		id: 1,
		started_at: '2026-11-24T10:00:00Z',
		status: 'done',
		limit: '2.0 GB',
		size_before: '2.0 GB',
		compacted: '2.0 GB',
		size_after: '1.6 GB',
		freed: '400.0 MB',
		stop: '1.6 GB',
		needed: '400.0 MB',
		deleted_steps: '5, 6',
		result: 'stopped in step 6',
		took: '1m',
		steps: []
	} as Run;

	it('is shown apart from what the run did', () => {
		expect(arrivedLine({ ...run, arrived: '100.0 MB', size_now: '1.7 GB' })).toBe(
			'Meanwhile 100.0 MB of new data arrived (the database held 1.7 GB at the end).'
		);
		expect(
			arrivedLine({
				...run,
				project: { id: 2, name: 'backend' },
				arrived: '1 MB',
				size_now: '9 MB'
			})
		).toContain('the project held 9 MB');
		expect(arrivedLine(run)).toBe('');
	});
});

describe('the data a run started with', () => {
	const run = {
		id: 1,
		started_at: '2026-11-24T10:00:00Z',
		status: 'done',
		limit: '2.0 GB',
		size_before: '2.0 GB',
		compacted: '2.0 GB',
		size_after: '1.6 GB',
		freed: '400.0 MB',
		stop: '1.6 GB',
		needed: '400.0 MB',
		deleted_steps: '5, 6',
		result: 'stopped in step 6',
		took: '1m',
		steps: []
	} as Run;

	it('reads with what the run left', () => {
		expect(heldLine(run)).toBe(
			'Data held: 2.0 GB before, so about 400.0 MB to free; 1.6 GB of it after (−400.0 MB).'
		);
	});

	it('is the files for a run recorded before it was measured apart', () => {
		expect(heldLine({ ...run, before_files: true, size_after: '', freed: '' })).toBe(
			'Files: 2.0 GB before, so about 400.0 MB to free (recorded before the data held was measured apart from the files).'
		);
	});
});
