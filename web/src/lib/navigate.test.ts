import { describe, expect, it, vi } from 'vitest';
import { showChanged } from './navigate.ts';

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: navigation.goto }));

describe('showChanged', () => {
	it('goes to the changed address from the first page, keeping the page state', () => {
		showChanged({ pathname: '/logs', search: '?level=error&page=3' }, { project: ['backend'] });

		expect(navigation.goto).toHaveBeenLastCalledWith('/logs?level=error&project=backend', {
			reset: false,
			replace: false
		});
	});

	it('replaces the history entry while typing', () => {
		showChanged({ pathname: '/logs', search: '' }, { query: 'timeout' }, true);

		expect(navigation.goto).toHaveBeenLastCalledWith('/logs?query=timeout', {
			reset: false,
			replace: true
		});
	});
});
