import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { PageProps } from './$types';
import Page from './+page.svelte';
import type { IssueDetails } from './issue.ts';

const api = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn(), invalidateAll: vi.fn() }));
vi.mock('$app/paths', () => ({
	resolve: (route: string, params: Record<string, string> = {}) =>
		route.replace('/(app)', '').replace('[id]', params.id ?? '')
}));
vi.mock('$app/state', () => ({
	page: { url: new URL('http://localhost/issues/aaaaaaaa'), data: { session: {} } }
}));
vi.mock('#lib/api/client.ts', async (actual) => ({
	...(await actual<typeof import('#lib/api/client.ts')>()),
	get: api.get
}));

function eventRow(id: string) {
	return { event_id: id.padEnd(32, '0'), received_at: '2026-11-25T12:00:00Z' };
}

function detailsOf(issueId: string, eventIds: string[]): IssueDetails {
	return {
		issue: {
			id: issueId,
			title: `Error of ${issueId}`,
			project: 'backend',
			level: 'error',
			status: 'unresolved',
			event_count: 100,
			stored_events: 100,
			first_seen: '2026-11-25T12:00:00Z',
			last_seen: '2026-11-25T12:00:00Z'
		},
		event_state: 'none',
		agent_prompt: '',
		events: { items: eventIds.map(eventRow), next_cursor: '20' }
	} as unknown as IssueDetails;
}

function shownEvents(): string[] {
	return screen
		.getAllByRole('link')
		.map((link) => link.textContent?.trim() ?? '')
		.filter((text) => /^[a-z]\d{7}$/.test(text));
}

describe('the issue page', () => {
	it('drops older events that come after the page loaded another issue', async () => {
		let answer: (page: unknown) => void = () => {};

		api.get.mockImplementation(() => new Promise((done) => (answer = done)));

		const data = { details: detailsOf('aaaaaaaa', ['a0000001']) } as PageProps['data'];
		const page = render(Page, { data } as PageProps);

		await userEvent.click(screen.getByRole('button', { name: 'Load more' }));
		await page.rerender({ data: { details: detailsOf('bbbbbbbb', ['b0000001']) } } as PageProps);
		answer({ items: [eventRow('a0000002')], next_cursor: '40' });

		await waitFor(() =>
			expect(screen.getByRole('button', { name: 'Load more' }).getAttribute('aria-busy')).toBeNull()
		);
		expect(shownEvents()).toEqual(['b0000001']);
	});
});
