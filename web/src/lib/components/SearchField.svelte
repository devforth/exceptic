<!--
@component
A search field that reports every change of its text (the list waits for changes to stop, see
#lib/navigate.svelte.ts), and on Enter with `now` to load at once; with text a × clears it at
once. In a FilterBar it takes the width the other filters leave (at least 200px).
-->
<script lang="ts">
	import { controlClasses } from './field.ts';

	type Props = { value: string; label: string; onchange: (value: string, now: boolean) => void };

	let { value, label, onchange }: Props = $props();

	let input: HTMLInputElement | undefined = $state();
</script>

<span class="relative flex min-w-[200px] flex-1">
	<input
		bind:this={input}
		type="search"
		class={[controlClasses, 'w-full pr-8 [&::-webkit-search-cancel-button]:appearance-none']}
		{value}
		placeholder={label}
		aria-label={label}
		oninput={(event) => onchange(event.currentTarget.value, false)}
		onkeydown={(event) => {
			if (event.key === 'Enter') onchange(event.currentTarget.value, true);
		}}
	/>
	{#if value}
		<button
			type="button"
			class="text-base absolute top-1/2 right-1.5 inline-flex size-6 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full leading-none text-muted focus-ring hover:bg-hover hover:text-text"
			title="Clear"
			aria-label="Clear {label}"
			onclick={() => {
				onchange('', true);
				input?.focus();
			}}
		>
			×
		</button>
	{/if}
</span>
