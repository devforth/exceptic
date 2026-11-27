import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Out } from '#lib/api/client.ts';
import ConnectedApps from './ConnectedApps.svelte';
import McpSetup from './McpSetup.svelte';

const app = vi.hoisted(() => ({
	goto: vi.fn(),
	invalidateAll: vi.fn(),
	send: vi.fn(),
	post: vi.fn(),
	url: new URL('http://localhost/agents?client=other')
}));

vi.mock('$app/navigation', () => ({ goto: app.goto, invalidateAll: app.invalidateAll }));
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return app.url;
		},
		state: {}
	}
}));
vi.mock('#lib/api/client.ts', async (original) => ({
	...(await original<typeof import('#lib/api/client.ts')>()),
	send: app.send,
	post: app.post
}));

const setup: Out<'get_mcp_setup'> = {
	url: 'http://example.com/mcp',
	server_name: 'exceptic',
	claude_command: 'claude mcp add --transport http exceptic http://example.com/mcp',
	codex_command: 'codex mcp add exceptic --url http://example.com/mcp',
	project_config: '{}',
	token_set: false,
	unencrypted: true,
	ips_restricted: false
};

const secret = 'emcp_pt_secret-value';

beforeEach(() => {
	vi.clearAllMocks();
	app.url = new URL('http://localhost/agents?client=other');
	app.send.mockResolvedValue({ token: secret, setup_prompt: `prompt with ${secret}` });
});

describe('McpSetup', () => {
	it('warns about plain HTTP and opens the tab the address names', () => {
		render(McpSetup, { setup, appName: 'Exceptic' });

		expect(screen.getByRole('alert').textContent).toContain('unencrypted');
		expect(screen.getByRole('tab', { name: 'Other' }).getAttribute('aria-selected')).toBe('true');
		expect(screen.queryByText(/A personal token is active/)).toBeNull();
	});

	it('shows a new token once, masked, with its setup prompt, and keeps it nowhere else', async () => {
		const stored = vi.spyOn(Storage.prototype, 'setItem');

		render(McpSetup, { setup, appName: 'Exceptic' });
		await userEvent.click(screen.getByRole('button', { name: 'Generate token' }));

		const field = await screen.findByRole('textbox', { name: 'Authorization header value' });

		expect(app.send).toHaveBeenCalledWith('personal_token');
		expect((field as HTMLInputElement).value).not.toContain(secret);
		expect(screen.getByRole('button', { name: /Copy setup prompt/ })).toBeTruthy();
		expect(screen.getByText(/Any previous personal token has stopped working/)).toBeTruthy();
		expect(app.invalidateAll).toHaveBeenCalled();
		expect(app.goto).not.toHaveBeenCalled();
		expect(stored).not.toHaveBeenCalled();
		expect(location.href).not.toContain(secret);
	});

	it('gives the endpoint as a command with a blue copy button, as on every other page', () => {
		render(McpSetup, { setup, appName: 'Exceptic' });

		expect(screen.queryByRole('textbox', { name: 'MCP endpoint' })).toBeNull();
		expect(screen.getByLabelText('MCP endpoint').textContent).toContain(setup.url);

		const copy = screen.getByTitle('Copy the endpoint');

		expect(copy.className).toContain('bg-info-bg');
		expect(copy.className).not.toContain('bg-code-bg');
	});

	it('asks before replacing a token', async () => {
		render(McpSetup, { setup: { ...setup, token_set: true }, appName: 'Exceptic' });

		expect(screen.getByText(/A personal token is active/)).toBeTruthy();
		await userEvent.click(screen.getByRole('button', { name: 'Regenerate token' }));
		expect(app.send).not.toHaveBeenCalled();

		await userEvent.click(screen.getByRole('button', { name: 'Regenerate' }));
		await waitFor(() => expect(app.send).toHaveBeenCalledWith('personal_token'));
	});
});

const agents: Out<'list_agents'> = {
	user: 'admin',
	user_id: 'viewer01',
	agents: [
		{
			grant_id: 'grant001',
			name: 'Codex <test>',
			kind: 'oauth',
			created_at: '2026-11-20T10:00:00Z',
			connected: true
		},
		{
			grant_id: 'grant002',
			name: 'Personal token',
			kind: 'token',
			created_at: '2026-11-20T10:00:00Z',
			connected: false
		}
	]
};

describe('ConnectedApps', () => {
	it('lists connected apps only, names as text', () => {
		render(ConnectedApps, { agents });

		expect(screen.getByText('Codex <test>')).toBeTruthy();
		expect(screen.getByText('Not used yet')).toBeTruthy();
		expect(screen.queryByText('Personal token')).toBeNull();
	});

	it('says when no app is connected, without an empty table', () => {
		render(ConnectedApps, { agents: { ...agents, agents: [agents.agents![1]] } });

		expect(screen.getByText('No apps connected yet.')).toBeTruthy();
		expect(screen.queryByRole('table')).toBeNull();
	});

	it('disconnects after a confirmation', async () => {
		app.post.mockResolvedValue({ done: 'disconnected' });
		render(ConnectedApps, { agents });

		await userEvent.click(screen.getByRole('button', { name: 'Disconnect' }));
		expect(app.post).not.toHaveBeenCalled();

		const dialog = screen.getByRole('dialog');

		expect(dialog.textContent).toContain('Disconnect Codex <test>?');
		await userEvent.click(
			[...dialog.querySelectorAll('button')].find(
				(button) => button.textContent?.trim() === 'Disconnect'
			)!
		);
		await waitFor(() =>
			expect(app.post).toHaveBeenCalledWith('disconnect_agent', { grant_id: 'grant001' })
		);
		expect(app.invalidateAll).toHaveBeenCalled();
	});
});
