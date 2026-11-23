<!--
@component
The "?" beside a field or section: its text in the tooltip, and as its name for screen
readers; it takes the focus, so the keyboard reaches the text too. `id`: for aria-describedby
of the field it explains.
-->
<script lang="ts">
	import Icon from './Icon.svelte';

	let { text, id }: { text: string; id?: string } = $props();
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<span
	{id}
	role="img"
	tabindex="0"
	title={text}
	aria-label={text}
	class="inline-flex shrink-0 cursor-help items-center justify-center rounded-full p-px text-muted opacity-75 transition-[color,opacity,background-color] duration-150 hover:bg-accent-soft hover:text-accent hover:opacity-100 focus-visible:bg-accent-soft focus-visible:text-accent focus-visible:opacity-100 focus-visible:shadow-[0_0_0_2px_var(--color-focus)] focus-visible:outline-hidden motion-reduce:transition-none"
>
	<Icon name="hint" size={15} />
</span>
