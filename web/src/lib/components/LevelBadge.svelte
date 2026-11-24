<!--
@component
An event or log level in the color of its group (shownLevel, generated from the server's rule).
-->
<script lang="ts">
	import { shownLevel } from '#lib/api/catalog.gen.ts';
	import Badge from './Badge.svelte';
	import type { BadgeTone } from './badge.ts';

	type Props = { level: string; href?: string; remove?: { href: string; label: string } };

	let { level, href, remove }: Props = $props();

	const groups: Record<string, BadgeTone | undefined> = shownLevel;
	// An unknown level counts as an error, as the server stores it.
	const tone = $derived(groups[level] ?? 'error');
</script>

<Badge {tone} label={level} {href} {remove} />
