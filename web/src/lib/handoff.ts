// A secret handed from one page to the next, once: a temporary password made on one page and
// shown on the user's. It stays in this module's memory only, never in the address, the
// history or storage, so a reload or Back does not show it again.
let held: { owner: string; secret: string } | undefined;

/** Keeps secret for the page of owner (an id). */
export function handOver(owner: string, secret: string): void {
	held = { owner, secret };
}

/** The secret kept for owner, once; any other owner gets nothing and it is dropped. */
export function takeOver(owner: string): string | undefined {
	const secret = held?.owner === owner ? held.secret : undefined;

	held = undefined;

	return secret;
}
