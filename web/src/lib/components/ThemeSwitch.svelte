<!--
@component
Dark mode, an item of the menu with a Switch: on when dark is chosen in this browser, or nothing
is and the system is dark (followed as it changes). Toggling stores light or dark. A choice made
in another tab applies here too.
-->
<script lang="ts">
	import { applyTheme, isDark, readTheme, saveTheme, systemDarkQuery } from '#lib/theme.ts';
	import { themeKey, type Theme } from '#lib/theme.ts';
	import NavLink from './NavLink.svelte';

	let { collapsed = false }: { collapsed?: boolean } = $props();

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

	function toggle() {
		const chosen = dark ? 'light' : 'dark';
		theme = chosen;
		saveTheme(chosen);
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
