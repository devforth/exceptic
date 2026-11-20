<!--
@component
A search field that reports what is typed once typing pauses.
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
	class={[controlClasses, 'w-48']}
	{value}
	placeholder={label}
	aria-label={label}
	oninput={(event) => typed(event.currentTarget.value)}
/>
