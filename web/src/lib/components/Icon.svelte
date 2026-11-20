<!-- An icon of the map in #lib/icons, drawn in the text color. Hidden from screen readers unless it has a label. -->
<script lang="ts">
	import { icons, type IconName } from '#lib/icons.ts';

	type Props = { name: IconName; size?: 14 | 16 | 18 | 24; label?: string };

	let { name, size = 18, label }: Props = $props();

	const Drawn = $derived(icons[name]);
</script>

{#if label}
	<Drawn {size} class="shrink-0" role="img" aria-label={label} />
{:else}
	<Drawn {size} class="shrink-0" />
{/if}
