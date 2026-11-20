// How the app writes times and log lines; until the old UI goes it repeats internal/ui/helpers.

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
