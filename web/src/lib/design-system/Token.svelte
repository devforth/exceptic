<!-- A color: its swatch (a class of the token, or a color), its name and its value under it. -->
<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { swatch?: string; color?: string; value: string; children: Snippet };

	let { swatch, color, value, children }: Props = $props();
</script>

<div class="flex items-center gap-2.5 text-xs">
	<span
		class={[
			'size-8 shrink-0 rounded-note shadow-[inset_0_0_0_1px_rgb(0_0_0/0.1)] forced-color-adjust-none',
			swatch
		]}
		style:background-color={color}
	></span>
	<span class="min-w-0">
		{@render children()}
		<code class="block text-xs text-muted">{value}</code>
	</span>
</div>
