import { describe, expect, it } from 'vitest';
import { projectId } from './params.ts';

describe('projectId', () => {
	it('takes positive numbers only', () => {
		expect(projectId('1')).toBe(1);
		expect(projectId('12345678')).toBe(12345678);

		for (const id of ['0', '-1', 'backend', '1.5', '', '01', 'new', 'cleanups']) {
			expect(projectId(id)).toBeUndefined();
		}
	});
});
