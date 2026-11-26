// The rules the notifications pages apply in the browser while the user types; the server
// checks every one of them again.
import { limits } from '#lib/api/catalog.gen.ts';
import { post } from '#lib/api/client.ts';
import type { Confirmation } from '#lib/components/confirm.ts';
import type { StatusTone } from '#lib/components/status.ts';
import { toast } from '#lib/toast.svelte.ts';

/** The question before a connection is deleted, from the list and from its page. */
export function deleteConfirmation(name: string): Confirmation {
	return {
		title: 'Delete connection',
		message: `Delete the connection ${name}? Its queued messages are dropped; a confirmed address stays confirmed.`,
		confirmLabel: 'Delete'
	};
}

export function pauseConnectionConfirmation(name: string): Confirmation {
	return {
		title: 'Pause notifications',
		message: `Pause the connection ${name}? Nothing is sent through it until it is resumed.`,
		confirmLabel: 'Pause'
	};
}

export async function deleteConnection(id: string): Promise<void> {
	await post('delete_connection', { connection_id: id });
	toast.show('Connection deleted.');
}

const typeLabels: Record<string, string> = {
	email: 'Email',
	telegram: 'Telegram',
	slack: 'Slack'
};

export function typeLabel(type: string): string {
	return typeLabels[type] ?? type;
}

/** What an empty name becomes: the start of the typed address, or the type's own. */
export function namePlaceholder(type: string, address: string, typePlaceholder: string): string {
	const local = type === 'email' ? (address.split('@')[0] ?? '').trim() : '';

	return local || typePlaceholder;
}

/**
 * Whether a chat's test can run: needs names its fields as groups ("a b|c": both a and b,
 * or c); a field counts when filled (a saved token counts too).
 */
export function canTest(needs: string, filled: (field: string) => boolean): boolean {
	return needs.split('|').some((group) => group.split(' ').every(filled));
}

const telegramToken = new RegExp(`^(?:${limits.telegramToken})$`, 'v');

/**
 * The bot's getUpdates, where the chat id is, for a token as typed: only one that matches
 * the token pattern, which leaves nothing to escape in the address.
 */
export function telegramUpdatesURL(token: string): string | undefined {
	const trimmed = token.trim();

	return telegramToken.test(trimmed)
		? `https://api.telegram.org/bot${trimmed}/getUpdates`
		: undefined;
}

/** Where confirming an address is: entering it, entering the mailed code, or confirmed. */
export type EmailStepState = {
	step: 'input' | 'code' | 'verified';
	address: string;
	notice: string;
	verifiedAt?: string;
};

/** The step after the server's answer to Verify, Confirm or Send the code again. */
export function emailStepAfter(answer: {
	email: string;
	state: string;
	notice?: string;
	verified_at?: string | null;
}): EmailStepState {
	return {
		step: answer.state === 'verified' ? 'verified' : 'code',
		address: answer.email,
		notice: answer.notice ?? '',
		verifiedAt: answer.verified_at ?? undefined
	};
}

/** The issue link box follows the type's default until the user sets it. */
export function issueLinkFor(typeDefault: boolean, touched: boolean, current: boolean): boolean {
	return touched ? current : typeDefault;
}

/** A connection's state in the list. */
export function connectionState(connection: { confirmed: boolean; active: boolean }): {
	label: string;
	tone: StatusTone;
} {
	if (!connection.confirmed) return { label: 'Awaiting confirmation', tone: 'warning' };

	return connection.active
		? { label: 'Active', tone: 'success' }
		: { label: 'Paused', tone: 'muted' };
}

type Health = {
	pending?: number;
	retrying?: number;
	last_sent_at?: string | null;
	last_error?: string;
	failing?: boolean;
};

/** How a connection's deliveries go, first match: failing, the last sent, queued, nothing. */
export function deliveryHealth(
	health: Health | null | undefined
):
	| { kind: 'failing'; error: string; retrying: number }
	| { kind: 'sent'; at: string }
	| { kind: 'queued'; count: number }
	| { kind: 'none' } {
	if (health?.failing) {
		return { kind: 'failing', error: health.last_error ?? '', retrying: health.retrying ?? 0 };
	}

	if (health?.last_sent_at) return { kind: 'sent', at: health.last_sent_at };
	if (health?.pending) return { kind: 'queued', count: health.pending };

	return { kind: 'none' };
}

const failingLength = 60;

/** A failing delivery's error, cut for the list (the whole one is its tooltip). */
export function shortError(error: string): string {
	const letters = [...error];

	return letters.length > failingLength ? `${letters.slice(0, failingLength).join('')}…` : error;
}

/** A message of the delivery log: retrying is a pending one that failed before. */
export function deliveryStatus(delivery: { status: string; retrying: boolean }): {
	label: string;
	tone: StatusTone;
} {
	if (delivery.retrying) return { label: 'retrying', tone: 'warning' };

	const tones: Record<string, StatusTone> = {
		pending: 'warning',
		sent: 'success',
		failed: 'error'
	};

	return { label: delivery.status, tone: tones[delivery.status] ?? 'muted' };
}
