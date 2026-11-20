import { describe, expect, it } from 'vitest';
import { movedIndex } from './roving.ts';

describe('movedIndex', () => {
	it('moves with the arrows of its direction and wraps around', () => {
		expect(movedIndex('ArrowRight', 0, 3, 'horizontal')).toBe(1);
		expect(movedIndex('ArrowRight', 2, 3, 'horizontal')).toBe(0);
		expect(movedIndex('ArrowLeft', 0, 3, 'horizontal')).toBe(2);
		expect(movedIndex('ArrowDown', 1, 3, 'vertical')).toBe(2);
		expect(movedIndex('ArrowUp', 1, 3, 'vertical')).toBe(0);
	});

	it('goes to the ends with Home and End', () => {
		expect(movedIndex('Home', 2, 3, 'vertical')).toBe(0);
		expect(movedIndex('End', 0, 3, 'horizontal')).toBe(2);
	});

	it('starts from either end when nothing has the focus yet', () => {
		expect(movedIndex('ArrowDown', -1, 3, 'vertical')).toBe(0);
		expect(movedIndex('ArrowUp', -1, 3, 'vertical')).toBe(2);
	});

	it('ignores the arrows of the other direction, other keys and empty groups', () => {
		expect(movedIndex('ArrowDown', 0, 3, 'horizontal')).toBeUndefined();
		expect(movedIndex('ArrowLeft', 0, 3, 'vertical')).toBeUndefined();
		expect(movedIndex('Enter', 0, 3, 'vertical')).toBeUndefined();
		expect(movedIndex('Home', -1, 0, 'vertical')).toBeUndefined();
	});
});
