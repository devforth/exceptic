import { describe, expect, it } from 'vitest';
import {
	canTest,
	connectionState,
	deliveryHealth,
	deliveryStatus,
	emailStepAfter,
	issueLinkFor,
	namePlaceholder,
	shortError,
	telegramUpdatesURL
} from './notifications.ts';

describe('notifications', () => {
	it('enables a chat test once a group of its fields is filled, a saved token counting', () => {
		const filledOf = (fields: string[]) => (field: string) => fields.includes(field);
		const slack = 'webhook_url|slack_bot_token slack_channel';

		expect(canTest('bot_token chat_id', filledOf(['bot_token']))).toBe(false);
		expect(canTest('bot_token chat_id', filledOf(['bot_token', 'chat_id']))).toBe(true);
		expect(canTest(slack, filledOf(['webhook_url']))).toBe(true);
		expect(canTest(slack, filledOf(['slack_bot_token']))).toBe(false);
		expect(canTest(slack, filledOf(['slack_bot_token', 'slack_channel']))).toBe(true);
	});

	it("links the bot's getUpdates only for a token of the right shape", () => {
		expect(telegramUpdatesURL(' 123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ ')).toBe(
			'https://api.telegram.org/bot123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ/getUpdates'
		);
		expect(telegramUpdatesURL('')).toBeUndefined();
		expect(telegramUpdatesURL('123456:short')).toBeUndefined();
		expect(telegramUpdatesURL('123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ/../x?y')).toBeUndefined();
	});

	it('names a connection after the start of its address, or as its type does', () => {
		expect(namePlaceholder('email', 'ops.team@example.com', 'team')).toBe('ops.team');
		expect(namePlaceholder('email', '', 'team')).toBe('team');
		expect(namePlaceholder('slack', 'ops@example.com', 'Slack')).toBe('Slack');
	});

	it('follows the type for the issue link until it is set by hand', () => {
		expect(issueLinkFor(false, false, true)).toBe(false);
		expect(issueLinkFor(false, true, true)).toBe(true);
	});

	it('moves the email step on with the answer', () => {
		expect(
			emailStepAfter({ email: 'ops@example.com', state: 'code', notice: 'We sent a code.' })
		).toEqual({ step: 'code', address: 'ops@example.com', notice: 'We sent a code.' });
		expect(
			emailStepAfter({
				email: 'ops@example.com',
				state: 'verified',
				verified_at: '2026-11-20T10:00:00Z'
			})
		).toEqual({
			step: 'verified',
			address: 'ops@example.com',
			notice: '',
			verifiedAt: '2026-11-20T10:00:00Z'
		});
	});

	it('says how deliveries go, failing first', () => {
		expect(
			deliveryHealth({ failing: true, last_error: 'boom', retrying: 2, last_sent_at: 'x' })
		).toEqual({ kind: 'failing', error: 'boom', retrying: 2 });
		expect(deliveryHealth({ last_sent_at: '2026-11-20T10:00:00Z', pending: 3 })).toEqual({
			kind: 'sent',
			at: '2026-11-20T10:00:00Z'
		});
		expect(deliveryHealth({ pending: 3 })).toEqual({ kind: 'queued', count: 3 });
		expect(deliveryHealth(null)).toEqual({ kind: 'none' });
	});

	it('cuts a long error by letters', () => {
		const error = 'slack: 500 ' + 'помилка '.repeat(10);

		expect([...shortError(error)]).toHaveLength(61);
		expect(shortError('short')).toBe('short');
	});

	it('tells retrying messages from waiting ones', () => {
		expect(deliveryStatus({ status: 'pending', retrying: true })).toEqual({
			label: 'retrying',
			tone: 'warning'
		});
		expect(deliveryStatus({ status: 'sent', retrying: false }).tone).toBe('success');
		expect(deliveryStatus({ status: 'failed', retrying: false }).tone).toBe('error');
	});

	it('shows a connection as awaiting, active or paused', () => {
		expect(connectionState({ confirmed: false, active: true }).label).toBe('Awaiting confirmation');
		expect(connectionState({ confirmed: true, active: true }).tone).toBe('success');
		expect(connectionState({ confirmed: true, active: false }).label).toBe('Paused');
	});
});
