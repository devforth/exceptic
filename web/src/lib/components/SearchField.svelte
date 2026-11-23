<!--
@component
A search field that reports what is typed once typing pauses; in a FilterBar it takes the
width the other filters leave (at least 200px).
-->
<script lang="ts">
	import { controlClasses } from './field.ts';

	type Props = { value: string; label: string; onchange: (value: string) => void };

	let { value, label, onchange }: Props = $props();

	let timer: ReturnType<typeof setTimeout> | undefined;

	function typed(text: string) {
		clearTimeout(timer);
		timer = setTimeout(() => onchange(text.trim()), 300);
	}
</script>

<input
	type="search"
	class={[controlClasses, 'min-w-[200px] flex-1']}
	{value}
	placeholder={label}
	aria-label={label}
	oninput={(event) => typed(event.currentTarget.value)}
/>
