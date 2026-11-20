import { describe, expect, it } from 'vitest';
import { cleanLog, exactTime, readableLog, timeAgo } from './format.ts';

const now = Date.parse('2026-11-20T12:00:00Z');

describe('timeAgo', () => {
	it('reads the distance back, then the date', () => {
		expect(timeAgo('2026-11-20T11:59:30Z', now)).toBe('just now');
		expect(timeAgo('2026-11-20T11:55:00Z', now)).toBe('5m ago');
		expect(timeAgo('2026-11-20T09:00:00Z', now)).toBe('3h ago');
		expect(timeAgo('2026-11-18T12:00:00Z', now)).toBe('2d ago');
		expect(timeAgo('2026-09-01T12:00:00Z', now)).toBe('2026-09-01');
		expect(timeAgo('2026-11-20T12:04:00Z', now)).toBe('in 4m');
	});

	it('gives the exact UTC time for a tooltip', () => {
		expect(exactTime('2026-11-17T12:14:41.242Z')).toBe('2026-11-17 12:14:41 UTC');
	});
});

describe('log lines', () => {
	it('drop terminal colors', () => {
		expect(cleanLog('\x1b[31merror\x1b[0m: failed')).toBe('error: failed');
	});

	it('print JSON fields as lines', () => {
		expect(readableLog('{"msg":"failed","err":"Error: x\\n    at f (a.js:1)","code":5}')).toBe(
			'msg: failed\n\nerr:\nError: x\n    at f (a.js:1)\n\ncode: 5'
		);
		expect(readableLog('plain text')).toBe('plain text');
		expect(readableLog('[1,2]')).toBe('[1,2]');
	});
});
