import { describe, expect, it } from 'vitest';
import { unavailable, type MenuItem } from './menu.ts';

describe('unavailable', () => {
	const item: MenuItem = { label: 'Delete', icon: 'delete', danger: true, action: () => {} };

	it('keeps the item without a reason', () => {
		expect(unavailable(item)).toBe(item);
	});

	it('disables it with the reason, keeping its look', () => {
		expect(unavailable(item, 'Not yours.')).toEqual({
			label: 'Delete',
			icon: 'delete',
			danger: true,
			separatorBefore: undefined,
			disabled: 'Not yours.'
		});
	});
});
