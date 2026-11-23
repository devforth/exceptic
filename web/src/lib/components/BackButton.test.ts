import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import BackButton from './BackButton.svelte';

let navigatedFrom: { url: URL } | null = null;

vi.mock('$app/navigation', () => ({
	afterNavigate: (callback: (navigation: { from: { url: URL } | null }) => void) =>
		callback({ from: navigatedFrom })
}));

afterEach(() => {
	navigatedFrom = null;
	vi.restoreAllMocks();
});

describe('BackButton', () => {
	it('links to the page above when the page was opened directly', () => {
		render(BackButton, { fallback: '/projects' });

		expect(screen.getByRole('link', { name: 'Back' }).getAttribute('href')).toBe('/projects');
	});

	it('goes back in the history after a navigation inside the app', async () => {
		navigatedFrom = { url: new URL('http://localhost/projects') };
		const back = vi.spyOn(history, 'back').mockImplementation(() => {});

		render(BackButton, { fallback: '/projects' });
		await userEvent.click(screen.getByRole('button', { name: 'Back' }));

		expect(back).toHaveBeenCalledOnce();
	});
});
