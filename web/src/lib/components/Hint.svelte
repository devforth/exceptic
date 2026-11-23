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
	class="inline-flex shrink-0 cursor-help rounded-full p-px text-muted opacity-75 focus-ring hover:bg-selected hover:text-accent hover:opacity-100 focus-visible:opacity-100"
>
	<Icon name="hint" size={14} />
</span>
