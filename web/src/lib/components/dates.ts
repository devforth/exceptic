// The days of DateRange.svelte: UTC days as YYYY-MM-DD, the values of the date filters
// (#lib/filters.ts), and the weeks of its calendar, Monday first.

const dayMs = 86_400_000;

export const monthNames = [
	'January',
	'February',
	'March',
	'April',
	'May',
	'June',
	'July',
	'August',
	'September',
	'October',
	'November',
	'December'
] as const;

export const weekdays = [
	'Monday',
	'Tuesday',
	'Wednesday',
	'Thursday',
	'Friday',
	'Saturday',
	'Sunday'
] as const;

const parse = (day: string) => Date.parse(`${day}T00:00:00Z`);
const format = (time: number) => new Date(time).toISOString().slice(0, 10);

/** The day some days later (or earlier, with a negative count). */
export function addDays(day: string, count: number): string {
	return format(parse(day) + count * dayMs);
}

/** The same day some months later or earlier, the month's last day when it has fewer days. */
export function addMonths(day: string, count: number): string {
	const date = new Date(parse(day));
	const month = date.getUTCMonth() + count;
	const last = new Date(Date.UTC(date.getUTCFullYear(), month + 1, 0)).getUTCDate();

	return format(Date.UTC(date.getUTCFullYear(), month, Math.min(date.getUTCDate(), last)));
}

/** The first day of the day's month. */
export function monthOf(day: string): string {
	return `${day.slice(0, 7)}-01`;
}

/** The day's place in its week: 0 Monday … 6 Sunday. */
export function weekdayOf(day: string): number {
	return (new Date(parse(day)).getUTCDay() + 6) % 7;
}

/** The weeks shown for a month, Monday first, with the days of the months around it. */
export function monthWeeks(month: string): string[][] {
	const first = monthOf(month);
	const start = addDays(first, -weekdayOf(first));
	const next = addMonths(first, 1);
	const weeks: string[][] = [];

	for (let week = start; week < next; week = addDays(week, 7)) {
		weeks.push(Array.from({ length: 7 }, (_, index) => addDays(week, index)));
	}

	return weeks;
}

/** "November 2026". */
export function monthTitle(month: string): string {
	return `${monthNames[Number(month.slice(5, 7)) - 1]} ${month.slice(0, 4)}`;
}

/** "Friday, 20 November 2026": the name of a day in the calendar. */
export function dayName(day: string): string {
	const [year, month, date] = day.split('-');

	return `${weekdays[weekdayOf(day)]}, ${Number(date)} ${monthNames[Number(month) - 1]} ${year}`;
}

/** "20 Nov", with the year when it is not this one. */
function shortDay(day: string, year: string): string {
	const text = `${Number(day.slice(8, 10))} ${monthNames[Number(day.slice(5, 7)) - 1].slice(0, 3)}`;

	return day.startsWith(year) ? text : `${text} ${day.slice(0, 4)}`;
}

/** The range in the field: "20 Nov – 23 Nov", "from 20 Nov", "until 23 Nov", or empty. */
export function rangeText(from: string, to: string, today: string): string {
	const year = today.slice(0, 4);

	if (from && to) {
		return from === to ? shortDay(from, year) : `${shortDay(from, year)} – ${shortDay(to, year)}`;
	}

	return from ? `from ${shortDay(from, year)}` : to ? `until ${shortDay(to, year)}` : '';
}
