import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ErrorPage from './ErrorPage.svelte';

const shown = vi.hoisted(() => ({ status: 404, message: '' }));

vi.mock('$app/state', () => ({
	page: {
		get status() {
			return shown.status;
		},
		get error() {
			return { message: shown.message };
		},
		data: {}
	}
}));

beforeEach(() => {
	shown.status = 404;
	shown.message = 'not found';
});

describe('ErrorPage', () => {
	it('tells a page that does not exist', () => {
		render(ErrorPage);

		expect(screen.getByRole('heading', { name: 'Page not found' })).toBeTruthy();
	});

	it('tells a page the viewer may not open apart, with the reason', () => {
		shown.status = 403;
		shown.message = 'issue k3x9q2mf is not in your projects';
		render(ErrorPage);

		expect(screen.getByRole('heading', { name: 'You do not have access to this' })).toBeTruthy();
		expect(screen.getByText('issue k3x9q2mf is not in your projects')).toBeTruthy();
		expect(screen.queryByText('Page not found')).toBeNull();
	});

	it('shows any other failure as its message', () => {
		shown.status = 500;
		shown.message = 'internal error';
		render(ErrorPage);

		expect(screen.getByText('internal error')).toBeTruthy();
	});
});
