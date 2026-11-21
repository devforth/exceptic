import { describe, expect, it } from 'vitest';
import { handOver, takeOver } from './handoff.ts';

describe('handoff', () => {
	it('gives the secret to its owner once', () => {
		handOver('k3x9q2mf', 'Z6R4SGSGAQYPH2TOBTAQBSUH6X');

		expect(takeOver('k3x9q2mf')).toBe('Z6R4SGSGAQYPH2TOBTAQBSUH6X');
		expect(takeOver('k3x9q2mf')).toBeUndefined();
	});

	it('drops it when another page asks', () => {
		handOver('k3x9q2mf', 'Z6R4SGSGAQYPH2TOBTAQBSUH6X');

		expect(takeOver('a1b2c3d4')).toBeUndefined();
		expect(takeOver('k3x9q2mf')).toBeUndefined();
	});
});
