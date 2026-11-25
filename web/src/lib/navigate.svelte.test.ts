import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { editPause, editing, showChanged, shown } from './navigate.svelte.ts';

const navigation = vi.hoisted(() => ({ goto: vi.fn(() => Promise.resolve()) }));
const state = vi.hoisted(() => ({ page: { url: new URL('http://localhost/logs') } }));

vi.mock('$app/navigation', () => ({ goto: navigation.goto }));
vi.mock('$app/state', () => ({ page: state.page }));

function at(address: string) {
	state.page.url = new URL(address, 'http://localhost');

	return state.page.url;
}

describe('showChanged', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		navigation.goto.mockClear();
	});

	afterEach(async () => {
		await vi.runAllTimersAsync();
		vi.useRealTimers();
	});

	it('shows a change at once and loads it once changes stop, from the first page', async () => {
		const url = at('/logs?level=error&page=3');

		showChanged(url, { project: ['backend'] });

		expect(shown('project', [])).toEqual(['backend']);
		expect(editing()).toBe(true);
		expect(navigation.goto).not.toHaveBeenCalled();

		vi.advanceTimersByTime(editPause);
		expect(navigation.goto).toHaveBeenLastCalledWith('/logs?level=error&project=backend', {
			reset: false,
			replace: false
		});
		await vi.runAllTimersAsync();
		expect(shown('project', ['frontend'])).toEqual(['frontend']);
		expect(editing()).toBe(false);
	});

	it('makes one request of changes that keep coming', () => {
		const url = at('/logs');

		showChanged(url, { project: ['backend'] });
		vi.advanceTimersByTime(editPause - 1);
		showChanged(url, { project: ['backend', 'frontend'] });
		vi.advanceTimersByTime(editPause - 1);
		showChanged(url, { query: 'time ' }, { typing: true });

		expect(shown('query', '')).toBe('time ');
		vi.advanceTimersByTime(editPause);
		expect(navigation.goto).toHaveBeenCalledTimes(1);
		expect(navigation.goto).toHaveBeenLastCalledWith(
			'/logs?project=backend&project=frontend&query=time',
			{ reset: false, replace: false }
		);
	});

	it('replaces the history entry while only typing, and loads at once on Enter', () => {
		const url = at('/logs');

		showChanged(url, { query: 'timeout' }, { typing: true });
		showChanged(url, { query: 'timeout' }, { typing: true, now: true });

		expect(navigation.goto).toHaveBeenCalledTimes(1);
		expect(navigation.goto).toHaveBeenLastCalledWith('/logs?query=timeout', {
			reset: false,
			replace: true
		});
	});

	it('shows what the control chose when the address writes it otherwise', () => {
		const url = at('/issues?status=all');

		showChanged(url, { status: null }, { shown: { status: 'unresolved' } });

		expect(shown('status', 'all')).toBe('unresolved');
	});

	it('drops its changes when another navigation went first', () => {
		const url = at('/logs');

		showChanged(url, { level: ['error'] });
		at('/logs?project=backend');
		vi.advanceTimersByTime(editPause);

		expect(navigation.goto).not.toHaveBeenCalled();
		expect(editing()).toBe(false);
	});

	it('drops its changes when the user went to another page with the same query', () => {
		const url = at('/issues');

		showChanged(url, { project: ['backend'] });
		at('/logs');

		expect(shown('project', [])).toEqual([]);
		expect(editing()).toBe(false);
		vi.advanceTimersByTime(editPause);
		expect(navigation.goto).not.toHaveBeenCalled();
	});

	it('loads nothing when the changes come back to where the list is', () => {
		const url = at('/logs?level=error');

		showChanged(url, { level: ['error', 'info'] });
		showChanged(url, { level: ['error'] });
		vi.advanceTimersByTime(editPause);

		expect(navigation.goto).not.toHaveBeenCalled();
	});
});
