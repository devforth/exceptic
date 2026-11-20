import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Tabs from './Tabs.svelte';

const navigation = vi.hoisted(() => ({ goto: vi.fn(), url: new URL('http://localhost/mcp') }));

vi.mock('$app/navigation', () => ({ goto: navigation.goto }));
vi.mock('$app/state', () => ({
	page: {
		get url() {
			return navigation.url;
		},
		state: {}
	}
}));

const panel = createRawSnippet((id: () => string) => ({ render: () => `<p>Panel ${id()}</p>` }));

const tabs = [
	{ id: 'claude', label: 'Claude Code' },
	{ id: 'codex', label: 'Codex' },
	{ id: 'other', label: 'Other' }
];

const tab = (name: string) => screen.getByRole('tab', { name });

beforeEach(() => {
	navigation.goto.mockClear();
	navigation.url = new URL('http://localhost/mcp');
});

describe('Tabs', () => {
	it('shows the first tab, the only one in the Tab order', () => {
		render(Tabs, { label: 'MCP client', tabs, panel });

		expect(tab('Claude Code').getAttribute('aria-selected')).toBe('true');
		expect(tab('Claude Code').tabIndex).toBe(0);
		expect(tab('Codex').tabIndex).toBe(-1);
		expect(screen.getByText('Panel claude')).toBeTruthy();
		expect(screen.queryByText('Panel codex')).toBeNull();
	});

	it('moves with the arrows (wrapping), Home and End', async () => {
		render(Tabs, { label: 'MCP client', tabs, panel });

		await userEvent.click(tab('Claude Code'));
		await userEvent.keyboard('{ArrowRight}');
		expect(document.activeElement).toBe(tab('Codex'));
		expect(screen.getByText('Panel codex')).toBeTruthy();

		await userEvent.keyboard('{ArrowRight}{ArrowRight}');
		expect(document.activeElement).toBe(tab('Claude Code'));

		await userEvent.keyboard('{ArrowLeft}');
		expect(tab('Other').getAttribute('aria-selected')).toBe('true');

		await userEvent.keyboard('{Home}');
		expect(document.activeElement).toBe(tab('Claude Code'));

		await userEvent.keyboard('{End}');
		expect(document.activeElement).toBe(tab('Other'));
	});

	it('opens the tab the address names and writes the chosen one to it, shallow', async () => {
		navigation.url = new URL('http://localhost/mcp?client=codex');
		render(Tabs, { label: 'MCP client', tabs, param: 'client', panel });

		expect(tab('Codex').getAttribute('aria-selected')).toBe('true');

		await userEvent.click(tab('Other'));

		const [url, options] = navigation.goto.mock.lastCall as [URL, object];

		expect(url.href).toBe('http://localhost/mcp?client=other');
		expect(options).toMatchObject({ shallow: true, replace: true });
	});

	it('ignores a tab the address names that does not exist', () => {
		navigation.url = new URL('http://localhost/mcp?client=nope');
		render(Tabs, { label: 'MCP client', tabs, param: 'client', panel });

		expect(tab('Claude Code').getAttribute('aria-selected')).toBe('true');
	});
});
