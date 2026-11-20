import { describe, expect, it } from 'vitest';
import { toggled } from './selection.ts';

describe('toggled', () => {
	const known = ['fatal', 'error', 'warning'] as const;

	it('adds a checked value in the known order', () => {
		expect(toggled(known, ['warning'], 'fatal', true)).toEqual(['fatal', 'warning']);
	});

	it('removes an unchecked value', () => {
		expect(toggled(known, ['fatal', 'warning'], 'fatal', false)).toEqual(['warning']);
	});

	it('drops values that are not known', () => {
		expect(toggled(['a', 'b'], ['c'], 'a', true)).toEqual(['a']);
	});
});
