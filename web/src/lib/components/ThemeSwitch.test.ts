import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { themeKey } from '#lib/theme.ts';
import ThemeSwitch from './ThemeSwitch.svelte';

// The system theme: a media query whose answer the test changes.
let systemDark = false;
const listeners = new Set<(event: MediaQueryListEvent) => void>();

function setSystemDark(dark: boolean) {
	systemDark = dark;
	for (const listener of listeners) listener({ matches: dark } as MediaQueryListEvent);
}

beforeEach(() => {
	systemDark = false;
	listeners.clear();
	vi.stubGlobal('matchMedia', (query: string) => ({
		media: query,
		get matches() {
			return systemDark;
		},
		addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) =>
			listeners.add(listener),
		removeEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) =>
			listeners.delete(listener)
	}));
});

afterEach(() => {
	vi.unstubAllGlobals();
	localStorage.clear();
	delete document.documentElement.dataset.theme;
});

const control = () => screen.getByRole('switch', { name: 'Dark mode' });

describe('ThemeSwitch', () => {
	it('shows a dark system as on while nothing is chosen, and sets nothing', () => {
		systemDark = true;
		render(ThemeSwitch);

		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(document.documentElement.hasAttribute('data-theme')).toBe(false);
		expect(localStorage.getItem(themeKey)).toBeNull();
	});

	it('stores light or dark when toggled', async () => {
		const user = userEvent.setup();
		systemDark = true;
		render(ThemeSwitch);

		await user.click(control());
		expect(control().getAttribute('aria-checked')).toBe('false');
		expect(localStorage.getItem(themeKey)).toBe('light');
		expect(document.documentElement.dataset.theme).toBe('light');

		await user.click(control());
		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(localStorage.getItem(themeKey)).toBe('dark');
		expect(document.documentElement.dataset.theme).toBe('dark');
	});

	it('follows the system while nothing is chosen, and only then', async () => {
		const user = userEvent.setup();
		render(ThemeSwitch);
		expect(control().getAttribute('aria-checked')).toBe('false');

		setSystemDark(true);
		await Promise.resolve();
		expect(control().getAttribute('aria-checked')).toBe('true');

		await user.click(control()); // light chosen
		setSystemDark(false);
		setSystemDark(true);
		await Promise.resolve();
		expect(control().getAttribute('aria-checked')).toBe('false');
	});

	it('follows a choice made in another tab', async () => {
		render(ThemeSwitch);

		localStorage.setItem(themeKey, 'dark');
		await fireEvent(window, new StorageEvent('storage', { key: themeKey, newValue: 'dark' }));
		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(document.documentElement.dataset.theme).toBe('dark');

		localStorage.removeItem(themeKey);
		await fireEvent(window, new StorageEvent('storage', { key: themeKey, newValue: null }));
		expect(control().getAttribute('aria-checked')).toBe('false');
		expect(document.documentElement.hasAttribute('data-theme')).toBe(false);
	});

	it('starts from the stored choice, a switch when collapsed too', () => {
		localStorage.setItem(themeKey, 'dark');
		render(ThemeSwitch, { collapsed: true });

		expect(control().getAttribute('aria-checked')).toBe('true');
		expect(control().title).toBe('Dark mode');
	});
});
