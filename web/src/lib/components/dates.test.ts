import { describe, expect, it } from 'vitest';
import {
	addDays,
	addMonths,
	dayName,
	monthTitle,
	monthWeeks,
	rangeText,
	weekdayOf
} from './dates.ts';

describe('dates', () => {
	it('moves by days and months across month and year ends', () => {
		expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
		expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
		expect(addMonths('2026-01-31', 1)).toBe('2026-02-28');
		expect(addMonths('2026-01-15', -1)).toBe('2025-12-15');
		expect(addMonths('2026-11-23', 12)).toBe('2027-11-23');
	});

	it('lays a month out in weeks from Monday, with the days around it', () => {
		const weeks = monthWeeks('2026-11-01');

		expect(weekdayOf('2026-11-01')).toBe(6);
		expect(weeks[0]).toEqual([
			'2026-10-26',
			'2026-10-27',
			'2026-10-28',
			'2026-10-29',
			'2026-10-30',
			'2026-10-31',
			'2026-11-01'
		]);
		expect(weeks.at(-1)?.at(-1)).toBe('2026-12-06');
		expect(weeks).toHaveLength(6);
		expect(monthWeeks('2027-02-01')).toHaveLength(4);
	});

	it('names months and days', () => {
		expect(monthTitle('2026-11-01')).toBe('November 2026');
		expect(dayName('2026-11-20')).toBe('Friday, 20 November 2026');
	});

	it('writes the range short, with the year only when it is not this one', () => {
		expect(rangeText('2026-11-20', '2026-11-23', '2026-11-23')).toBe('20 Nov – 23 Nov');
		expect(rangeText('2025-12-30', '2026-01-02', '2026-11-23')).toBe('30 Dec 2025 – 2 Jan');
		expect(rangeText('2026-11-20', '2026-11-20', '2026-11-23')).toBe('20 Nov');
		expect(rangeText('2026-11-20', '', '2026-11-23')).toBe('from 20 Nov');
		expect(rangeText('', '2026-11-23', '2026-11-23')).toBe('until 23 Nov');
		expect(rangeText('', '', '2026-11-23')).toBe('');
	});
});
