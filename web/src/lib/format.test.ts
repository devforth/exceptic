import { describe, expect, it } from 'vitest';
import {
	sharePercent,
	cleanLog,
	clockTime,
	exactTime,
	formatSize,
	readableLog,
	shortDate,
	timeAgo
} from './format.ts';

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

	it('gives the time of day to the millisecond, in UTC', () => {
		expect(clockTime('2026-11-10T18:26:59.148+02:00')).toBe('16:26:59.148');
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

describe('formatSize', () => {
	it('writes sizes as the server does', () => {
		expect(formatSize(2 * 2 ** 30)).toBe('2.0 GB');
		expect(formatSize(0.4 * 2 * 2 ** 30)).toBe('819.2 MB');
		expect(formatSize(512)).toBe('512 B');
	});
});

describe('shortDate', () => {
	it('writes the UTC day and month', () => {
		expect(shortDate('2026-11-10T23:30:00Z')).toBe('10 Nov');
	});
});

describe('sharePercent', () => {
	it('says <1% for a part above nothing and 0% only for nothing', () => {
		expect(sharePercent(0, 2e9)).toBe('0%');
		expect(sharePercent(2.8e6, 2e9)).toBe('<1%');
		expect(sharePercent(1e9, 2e9)).toBe('50%');
	});

	it('never shows 100% before the whole is reached, nor more than it is', () => {
		expect(sharePercent(1.999e9, 2e9)).toBe('>99%');
		expect(sharePercent(2e9, 2e9)).toBe('100%');
		expect(sharePercent(2.5e9, 2e9)).toBe('125%');
	});

	it('shows nothing without a whole', () => {
		expect(sharePercent(10, 0)).toBe('');
	});
});
