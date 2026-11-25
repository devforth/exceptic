// How the app writes times and log lines.

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

/** "just now", "5m ago", "3h ago", "2d ago", then the date; a time to come reads "in 4m". */
export function timeAgo(iso: string, now = Date.now()): string {
	const elapsed = now - Date.parse(iso);

	if (elapsed < -minute) {
		// The same distance back.
		return 'in ' + timeAgo(new Date(now + elapsed).toISOString(), now).replace(/ ago$/, '');
	}

	if (elapsed < minute) return 'just now';
	if (elapsed < hour) return `${Math.floor(elapsed / minute)}m ago`;
	if (elapsed < day) return `${Math.floor(elapsed / hour)}h ago`;
	if (elapsed < 30 * day) return `${Math.floor(elapsed / day)}d ago`;

	return iso.slice(0, 10);
}

/** The exact time, for a tooltip: "2026-11-17 12:14:41 UTC". */
export function exactTime(iso: string): string {
	return new Date(iso)
		.toISOString()
		.replace('T', ' ')
		.replace(/\.\d+Z$/, ' UTC');
}

/** The time of day in UTC to the millisecond, for steps close together: "16:26:59.148". */
export function clockTime(iso: string): string {
	return new Date(iso).toISOString().slice(11, 23);
}

// eslint-disable-next-line no-control-regex -- the escape character is what it removes
const ansiEscape = /\x1b\[[0-9;]*[A-Za-z]/g;

/** A log line without the color codes of terminals. */
export function cleanLog(text: string): string {
	return text.replace(ansiEscape, '');
}

/**
 * A log line made easier to read: loggers like pino send errors as JSON with the stack
 * trace in one string full of "\n". Multi-line string fields become real lines, other
 * fields indented JSON; anything else stays as it is.
 */
export function readableLog(text: string): string {
	let value: unknown;

	try {
		value = JSON.parse(text);
	} catch {
		return text;
	}

	if (value === null || typeof value !== 'object' || Array.isArray(value)) {
		return text;
	}

	return Object.entries(value)
		.map(([key, field]) =>
			typeof field === 'string'
				? field.includes('\n')
					? `${key}:\n${field}`
					: `${key}: ${field}`
				: `${key}: ${JSON.stringify(field, null, 2)}`
		)
		.join('\n\n');
}

const sizeUnits = [
	{ suffix: 'TB', bytes: 2 ** 40 },
	{ suffix: 'GB', bytes: 2 ** 30 },
	{ suffix: 'MB', bytes: 2 ** 20 },
	{ suffix: 'KB', bytes: 2 ** 10 }
];

/** A byte count as the server writes sizes (config.FormatSize): "2.0 GB", "512 B". */
export function formatSize(bytes: number): string {
	const unit = sizeUnits.find((entry) => bytes >= entry.bytes);

	return unit ? `${(bytes / unit.bytes).toFixed(1)} ${unit.suffix}` : `${Math.round(bytes)} B`;
}

/** A day as "10 Nov": in UTC, or in the viewer's time zone when `local`. */
export function shortDate(iso: string, local = false): string {
	return new Date(iso).toLocaleDateString('en-GB', {
		day: 'numeric',
		month: 'short',
		timeZone: local ? undefined : 'UTC'
	});
}

/**
 * part of whole in whole percent: "<1%" for a part above nothing that rounds to 0, ">99%" for
 * one under the whole that rounds to 100; nothing without a whole.
 */
export function sharePercent(part: number, whole: number): string {
	if (whole <= 0) return '';

	const exact = (part * 100) / whole;

	if (part > 0 && exact < 1) return '<1%';
	if (part < whole && exact > 99) return '>99%';

	return `${Math.round(exact)}%`;
}

/** A count of a kind, "50,000 logs" or "1 envelope". */
export function countOf(count: number, kind: string): string {
	return `${count.toLocaleString('en-US')} ${count === 1 ? kind.replace(/s$/, '') : kind}`;
}
