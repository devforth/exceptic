import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { limits } from '#lib/api/catalog.gen.ts';
import { ApiError } from '#lib/api/client.ts';
import ConnectionForm from './ConnectionForm.svelte';

const api = vi.hoisted(() => ({ post: vi.fn() }));

vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<typeof import('#lib/api/client.ts')>()),
	post: api.post
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll: vi.fn() }));
vi.mock('$app/paths', () => ({
	resolve: (route: string, params?: { id?: string }) =>
		route.replace('/(app)', '').replace('[id=connectionId]', params?.id ?? '')
}));
vi.mock('$app/state', () => ({ page: { data: {} } }));

const type = (name: string, available = true) => ({
	type: name,
	available,
	reason: available ? undefined : 'set SMTP_HOST and EMAIL_FROM',
	include_issue_link: name === 'email',
	name_placeholder: name === 'slack' ? 'Slack' : 'team'
});

const defaults = (issueLinks: boolean) => ({
	types: [type('email'), type('telegram'), type('slack')],
	default_type: 'email',
	levels: ['error', 'fatal'],
	issue_links_available: issueLinks
});

const projects = [
	{ id: 1, name: 'backend' },
	{ id: 2, name: 'frontend' }
];

const field = (label: RegExp) => screen.getByLabelText<HTMLInputElement>(label);

beforeEach(() => api.post.mockReset());

describe('ConnectionForm', () => {
	it('locks a new email connection until its address is confirmed', () => {
		const { container } = render(ConnectionForm, { defaults: defaults(true), projects });

		expect(container.querySelector('fieldset[disabled]')).not.toBeNull();
		expect(field(/^Name/).matches(':disabled')).toBe(true);
		expect(screen.getByRole('checkbox', { name: /Include Open issue link/ })).toBeTruthy();
	});

	it('checks the chat fields with the server patterns and links getUpdates for a valid token', async () => {
		const user = userEvent.setup();

		render(ConnectionForm, { defaults: defaults(false), projects });
		await user.click(screen.getByRole('radio', { name: 'Telegram' }));

		const token = field(/^Bot token/);

		expect(token.pattern).toBe(limits.telegramToken);
		expect(token.type).toBe('password');
		expect(token.autocomplete).toBe('new-password');
		expect(field(/^Chat ID/).pattern).toBe(limits.telegramChat);
		expect(screen.getByRole('button', { name: 'Find chat ID' })).toHaveProperty('disabled', true);
		expect(screen.getByRole('button', { name: 'Send test message' })).toHaveProperty(
			'disabled',
			true
		);

		await user.type(token, '123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ');
		await user.type(field(/^Chat ID/), '-100');

		const link = screen.getByRole('link', { name: 'Find chat ID' });

		expect(link.getAttribute('href')).toBe(
			'https://api.telegram.org/bot123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ/getUpdates'
		);
		expect(link.getAttribute('rel')).toBe('noopener noreferrer');
		expect(screen.getByRole('button', { name: 'Send test message' })).toHaveProperty(
			'disabled',
			false
		);
		// Without PUBLIC_URL no issue link can be built: the option is not offered.
		expect(screen.queryByRole('checkbox', { name: /Include Open issue link/ })).toBeNull();
	});

	it('sends only the typed chat fields and shows a refusal at its field', async () => {
		const user = userEvent.setup();

		api.post.mockRejectedValueOnce(
			new ApiError(400, 'chat_id: wrong', { fields: { chat_id: 'Enter the chat ID.' } })
		);
		render(ConnectionForm, { defaults: defaults(false), projects });
		await user.click(screen.getByRole('radio', { name: 'Telegram' }));
		await user.type(field(/^Bot token/), '123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ');
		await user.type(field(/^Chat ID/), '-100');
		await user.click(screen.getByRole('checkbox', { name: /All projects/ }));
		await user.click(screen.getByRole('checkbox', { name: 'frontend' }));
		await user.click(screen.getByRole('button', { name: 'Add' }));

		await waitFor(() => expect(screen.getByText('Enter the chat ID.')).toBeTruthy());
		expect(api.post).toHaveBeenCalledWith('create_connection', {
			type: 'telegram',
			name: '',
			all_projects: false,
			projects: ['2'],
			levels: ['fatal', 'error'],
			active: true,
			bot_token: '123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ',
			chat_id: '-100'
		});
	});

	it('keeps saved tokens out of the page and the request', async () => {
		const user = userEvent.setup();

		api.post.mockResolvedValue({});
		render(ConnectionForm, {
			defaults: defaults(true),
			projects,
			connection: {
				id: 'abcd1234',
				name: 'Team chat',
				type: 'slack',
				confirmed: true,
				all_projects: true,
				project_ids: [],
				levels: ['error'],
				active: true,
				include_issue_link: false,
				saved_webhook_url: 'https://hoo…/x',
				secret_readable: true,
				retrying: 0
			}
		});

		const webhook = field(/^Incoming webhook URL/);

		expect(webhook.value).toBe('');
		expect(webhook.required).toBe(false);
		expect(screen.getByText('https://hoo…/x')).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Send test message' })).toHaveProperty(
			'disabled',
			false
		);

		await user.click(screen.getByRole('button', { name: 'Save' }));

		await waitFor(() =>
			expect(api.post).toHaveBeenCalledWith('update_connection', {
				connection_id: 'abcd1234',
				name: 'Team chat',
				all_projects: true,
				projects: [],
				levels: ['error'],
				active: true,
				include_issue_link: false
			})
		);
	});
});
