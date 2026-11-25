import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { themeKey } from '#lib/theme.ts';
import ThemeSwitch from './ThemeSwitch.svelte';

const post = vi.hoisted(() => vi.fn(() => Promise.resolve({})));

vi.mock('#lib/api/client.ts', () => ({ post }));

// The system theme: a media query whose answer the test changes.
let systemDark = false;

beforeEach(() => {
	systemDark = false;
	post.mockClear();
	vi.stubGlobal('matchMedia', (query: string) => ({
		media: query,
		get matches() {
			return systemDark;
		},
		addEventListener: () => {},
		removeEventListener: () => {}
	}));
});

afterEach(() => {
	vi.unstubAllGlobals();
	localStorage.clear();
	delete document.documentElement.dataset.theme;
});

const control = () => screen.getByRole('switch', { name: 'Dark mode' });

describe('ThemeSwitch', () => {
	it('records the theme shown at the first login: the system one', () => {
		systemDark = true;
		render(ThemeSwitch);

		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(post).toHaveBeenCalledWith('set_theme', { theme: 'dark' });
		expect(document.documentElement.dataset.theme).toBe('dark');
		expect(localStorage.getItem(themeKey)).toBe('dark');
	});

	it('records the one chosen in this browser before the first login', () => {
		systemDark = true;
		localStorage.setItem(themeKey, 'light');
		render(ThemeSwitch);

		expect(post).toHaveBeenCalledWith('set_theme', { theme: 'light' });
	});

	it('shows the user’s saved theme whatever this browser had, and records nothing', () => {
		localStorage.setItem(themeKey, 'light');
		render(ThemeSwitch, { saved: 'dark' });

		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(document.documentElement.dataset.theme).toBe('dark');
		expect(localStorage.getItem(themeKey)).toBe('dark');
		expect(post).not.toHaveBeenCalled();
	});

	it('saves light or dark when toggled, here and on the server', async () => {
		const user = userEvent.setup();
		render(ThemeSwitch, { saved: 'dark' });

		await user.click(control());
		expect(control().getAttribute('aria-checked')).toBe('false');
		expect(localStorage.getItem(themeKey)).toBe('light');
		expect(document.documentElement.dataset.theme).toBe('light');
		expect(post).toHaveBeenLastCalledWith('set_theme', { theme: 'light' });

		await user.click(control());
		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(post).toHaveBeenLastCalledWith('set_theme', { theme: 'dark' });
	});

	it('follows a choice made in another tab', async () => {
		render(ThemeSwitch, { saved: 'light' });

		localStorage.setItem(themeKey, 'dark');
		await fireEvent(window, new StorageEvent('storage', { key: themeKey, newValue: 'dark' }));
		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(document.documentElement.dataset.theme).toBe('dark');
	});

	it('is a switch when collapsed too', () => {
		render(ThemeSwitch, { collapsed: true, saved: 'dark' });

		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(control().title).toBe('Dark mode');
	});
});
