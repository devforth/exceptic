import { describe, expect, it } from 'vitest';
import { limits } from './catalog.gen.ts';

// Browsers read a pattern attribute as ^(?:pattern)$ in the "v" mode.
function matches(pattern: string, value: string): boolean {
	return new RegExp(`^(?:${pattern})$`, 'v').test(value);
}

describe('limits', () => {
	it('keep the patterns as the server wrote them, valid for a pattern attribute', () => {
		expect(matches(limits.projectNamePattern, 'backend-2')).toBe(true);
		expect(matches(limits.projectNamePattern, 'Not Valid')).toBe(false);
		expect(matches(limits.projectNamePattern, '123')).toBe(false);
		expect(matches(limits.usernamePattern, 'dev.ops@example-1')).toBe(true);
		expect(matches(limits.usernamePattern, 'two words')).toBe(false);
		expect(matches(limits.telegramToken, '123456:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw')).toBe(true);
		expect(matches(limits.telegramChat, '-1001234567890')).toBe(true);
		expect(matches(limits.slackToken, 'xoxb-1234567890-abcdef')).toBe(true);
		expect(matches(limits.slackChannel, '#alerts-prod')).toBe(true);
		expect(matches(limits.slackChannel, 'two words')).toBe(false);
	});
});
