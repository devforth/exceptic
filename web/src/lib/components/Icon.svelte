<!-- An icon of the map in #lib/icons, drawn in the text color and hidden from screen readers. -->
<script lang="ts">
	import { icons, type IconName } from '#lib/icons.ts';

	type Props = { name: IconName; size?: 14 | 15 | 16 | 18 | 24 };

	let { name, size = 18 }: Props = $props();

	const Drawn = $derived(icons[name]);
</script>

<Drawn {size} class="shrink-0" />
