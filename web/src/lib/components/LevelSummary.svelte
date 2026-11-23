<!--
@component
Chosen levels as LevelBadges, highest first (every one of them, also when all are chosen); a
muted "none" when none is.
-->
<script lang="ts">
	import { levels } from '#lib/api/catalog.gen.ts';
	import LevelBadge from './LevelBadge.svelte';

	type Props = { kind: keyof typeof levels; chosen: readonly string[] };

	let { kind, chosen }: Props = $props();

	const shown = $derived(
		(levels[kind] as readonly string[]).filter((level) => chosen.includes(level))
	);
</script>

<span class="inline-flex flex-wrap items-center gap-1">
	{#each shown as level (level)}
		<LevelBadge {level} />
	{:else}
		<span class="text-muted">none</span>
	{/each}
</span>
