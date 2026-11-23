import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import ConnectStatus from './ConnectStatus.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, state: {} } }));

describe('ConnectStatus', () => {
	it('waits for an app with the project key, with a way out', () => {
		render(ConnectStatus, { name: 'backend', connected: false, later: '/projects' });

		const status = screen.getByRole('status');

		expect(status.textContent).toContain('Waiting for backend to connect…');
		expect(status.textContent).toContain('This page updates by itself.');
		expect(screen.getByRole('link', { name: 'Do it later' }).getAttribute('href')).toBe(
			'/projects'
		);
		expect(screen.queryByRole('link', { name: /issues/i })).toBeNull();
	});

	it('says the project is connected and links to its issues and logs', () => {
		render(ConnectStatus, { name: 'backend', connected: true, later: '/projects' });

		expect(screen.getByRole('status').textContent).toContain('backend is connected');
		expect(screen.queryByText(/Waiting for/)).toBeNull();
		expect(screen.queryByRole('link', { name: 'Do it later' })).toBeNull();
		expect(screen.getByRole('link', { name: 'Open issues' }).getAttribute('href')).toBe(
			'/issues?project=backend'
		);
		expect(screen.getByRole('link', { name: 'Open logs' }).getAttribute('href')).toBe(
			'/logs?project=backend'
		);
	});
});
