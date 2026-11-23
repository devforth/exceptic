import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { applyTheme, isDark, readTheme, saveTheme, themeKey } from './theme.ts';

afterEach(() => {
	localStorage.clear();
	delete document.documentElement.dataset.theme;
	vi.restoreAllMocks();
});

describe('theme', () => {
	it('follows the system by default', () => {
		expect(readTheme()).toBe('system');
	});

	it('ignores a stored value it does not know', () => {
		localStorage.setItem(themeKey, 'sepia');

		expect(readTheme()).toBe('system');
	});

	it('stores and applies each choice', () => {
		saveTheme('dark');
		expect(readTheme()).toBe('dark');
		expect(document.documentElement.dataset.theme).toBe('dark');

		saveTheme('light');
		expect(readTheme()).toBe('light');
		expect(document.documentElement.dataset.theme).toBe('light');
	});

	it('applies the choice when storage is blocked', () => {
		vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
			throw new Error('blocked');
		});
		vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
			throw new Error('blocked');
		});

		expect(readTheme()).toBe('system');
		expect(() => saveTheme('dark')).not.toThrow();
		expect(document.documentElement.dataset.theme).toBe('dark');
	});

	it('is dark when dark is chosen, or nothing is and the system is dark', () => {
		expect(isDark('dark', false)).toBe(true);
		expect(isDark('light', true)).toBe(false);
		expect(isDark('system', true)).toBe(true);
		expect(isDark('system', false)).toBe(false);
	});

	it('clears the choice when the system theme is applied', () => {
		applyTheme('dark');
		applyTheme('system');

		expect(document.documentElement.hasAttribute('data-theme')).toBe(false);
	});

	it('applies a choice without storing it', () => {
		applyTheme('light');

		expect(document.documentElement.dataset.theme).toBe('light');
		expect(localStorage.getItem(themeKey)).toBeNull();
	});
});

// The page applies the stored choice before the app starts, so the first paint has its colors.
describe('the page and the tokens', () => {
	it('reads the same key in app.html before the first paint', () => {
		const page = readFileSync('src/app.html', 'utf8');

		expect(page).toContain(`localStorage.getItem('${themeKey}')`);
		expect(page).toContain('dataset.theme');
	});

	it('defines the dark values for a chosen dark theme and the system one', () => {
		const tokens = readFileSync('src/routes/layout.css', 'utf8');

		expect(tokens).toContain(`&[data-theme='dark']`);
		expect(tokens).toMatch(
			/@media \(prefers-color-scheme: dark\)\s*{\s*&:not\(\[data-theme='light'\]\)/
		);
		expect(tokens).toMatch(/@variant dark\s*{[^}]*--color-bg:/);
	});
});
