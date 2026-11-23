// The color theme chosen in this browser: light or dark, or none (the system's, no data-theme
// on <html>). app.html applies the stored choice before the first paint, with the same key.
export type Theme = 'system' | 'light' | 'dark';

export const themeKey = 'exceptic-theme';

export const systemDarkQuery = '(prefers-color-scheme: dark)';

export function readTheme(): Theme {
	try {
		const stored = localStorage.getItem(themeKey);

		return stored === 'light' || stored === 'dark' ? stored : 'system';
	} catch {
		return 'system'; // storage blocked
	}
}

export function isDark(theme: Theme, systemDark: boolean) {
	return theme === 'system' ? systemDark : theme === 'dark';
}

export function applyTheme(theme: Theme) {
	const root = document.documentElement;

	if (theme === 'system') delete root.dataset.theme;
	else root.dataset.theme = theme;
}

// Applied either way; without storage the choice lasts for this page.
export function saveTheme(theme: 'light' | 'dark') {
	applyTheme(theme);

	try {
		localStorage.setItem(themeKey, theme);
	} catch {
		// storage blocked
	}
}
