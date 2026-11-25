<!--
@component
Dark mode, an item of the menu with a Switch. The user's theme is saved on the server (saved,
from get_session), so every browser they log in from shows it; until then the theme they see (the
system's, or one chosen in this browser) is saved at once. This browser keeps a copy for the first
paint, and a choice made in another tab applies here too.
-->
<script lang="ts">
	import {
		applyTheme,
		isDark,
		readTheme,
		saveTheme,
		systemDarkQuery,
		themeKey,
		type Theme
	} from '#lib/theme.ts';
	import { post } from '#lib/api/client.ts';
	import NavLink from './NavLink.svelte';

	let { collapsed = false, saved = '' }: { collapsed?: boolean; saved?: string } = $props();

	const systemTheme = typeof matchMedia === 'function' ? matchMedia(systemDarkQuery) : undefined;

	let theme = $state<Theme>(readTheme());
	let systemDark = $state(systemTheme?.matches ?? false);

	const dark = $derived(isDark(theme, systemDark));

	$effect(() => {
		if (!systemTheme) return;

		const follow = (event: MediaQueryListEvent) => (systemDark = event.matches);
		systemTheme.addEventListener('change', follow);

		return () => systemTheme.removeEventListener('change', follow);
	});

	// The server's theme wins; without one, the theme shown now is recorded (the first login).
	$effect(() => {
		if (saved === 'light' || saved === 'dark') {
			theme = saved;
			saveTheme(saved);
		} else {
			choose(dark ? 'dark' : 'light');
		}
	});

	function choose(chosen: 'light' | 'dark') {
		theme = chosen;
		saveTheme(chosen);
		// The theme already applies here: one the server failed to keep is asked again at the next login.
		post('set_theme', { theme: chosen }).catch(() => {});
	}

	function toggle() {
		choose(dark ? 'light' : 'dark');
	}

	// A choice made in another tab of this browser (key null: storage cleared).
	function followOtherTab(event: StorageEvent) {
		if (event.key !== themeKey && event.key !== null) return;

		theme = readTheme();
		applyTheme(theme);
	}
</script>

<svelte:window onstorage={followOtherTab} />

<NavLink label="Dark mode" icon="themeDark" checked={dark} onclick={toggle} {collapsed} />
